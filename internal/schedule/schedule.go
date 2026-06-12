// Package schedule gates orchestrator dispatch to configured time windows
// and budgets, turning "weekend autonomy" from a manual ritual into config:
// runs start only inside the window, stop when the issue/token budget is
// spent, and each window close yields a summary for notifications.
package schedule

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Config mirrors the workflow schedule section.
type Config struct {
	// Windows are daily "HH:MM-HH:MM" spans (24h clock). A window crossing
	// midnight (e.g. "22:00-06:00") belongs to the day it starts. Empty
	// means all day.
	Windows []string
	// Days restricts windows to weekdays ("sat", "sunday", ...). Empty
	// means every day.
	Days []string
	// MaxIssues caps agent runs started per window (0 = unlimited).
	MaxIssues int
	// MaxTokens caps total tokens (in+out) consumed per window
	// (0 = unlimited). Checked before dispatch, so the cap can overshoot by
	// the runs already in flight.
	MaxTokens int64
}

type window struct {
	start int // minutes since midnight, inclusive
	end   int // minutes since midnight, exclusive; end <= start wraps past midnight
}

// Schedule implements the orchestrator's dispatch gate.
type Schedule struct {
	windows   []window
	days      map[time.Weekday]bool
	maxIssues int
	maxTokens int64

	mu              sync.Mutex
	active          bool
	windowOpenedAt  time.Time
	issuesStarted   int
	succeeded       int
	failed          int
	tokensUsed      int64
	exhaustedLogged bool
}

var dayNames = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

func New(cfg Config) (*Schedule, error) {
	s := &Schedule{
		maxIssues: cfg.MaxIssues,
		maxTokens: cfg.MaxTokens,
	}

	for _, raw := range cfg.Windows {
		w, err := parseWindow(raw)
		if err != nil {
			return nil, err
		}
		s.windows = append(s.windows, w)
	}

	if len(cfg.Days) > 0 {
		s.days = make(map[time.Weekday]bool, len(cfg.Days))
		for _, raw := range cfg.Days {
			day, ok := dayNames[strings.ToLower(strings.TrimSpace(raw))]
			if !ok {
				return nil, fmt.Errorf("invalid schedule day %q (use sun..sat or full names)", raw)
			}
			s.days[day] = true
		}
	}

	return s, nil
}

func parseWindow(raw string) (window, error) {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 2 {
		return window{}, fmt.Errorf("invalid schedule window %q (expected HH:MM-HH:MM)", raw)
	}
	start, err := parseClock(parts[0])
	if err != nil {
		return window{}, fmt.Errorf("invalid schedule window %q: %w", raw, err)
	}
	end, err := parseClock(parts[1])
	if err != nil {
		return window{}, fmt.Errorf("invalid schedule window %q: %w", raw, err)
	}
	if start == end {
		return window{}, fmt.Errorf("invalid schedule window %q: start and end are equal", raw)
	}
	return window{start: start, end: end}, nil
}

func parseClock(raw string) (int, error) {
	var hours, minutes int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d:%d", &hours, &minutes); err != nil {
		return 0, fmt.Errorf("bad time %q", raw)
	}
	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("bad time %q", raw)
	}
	return hours*60 + minutes, nil
}

// activeAt reports whether t falls inside the schedule. Overnight windows
// match when either the evening part of an allowed day or the morning
// spill-over of the previous allowed day contains t.
func (s *Schedule) activeAt(t time.Time) bool {
	if len(s.windows) == 0 {
		return s.dayAllowed(t.Weekday())
	}

	minute := t.Hour()*60 + t.Minute()
	for _, w := range s.windows {
		if w.start < w.end {
			if minute >= w.start && minute < w.end && s.dayAllowed(t.Weekday()) {
				return true
			}
			continue
		}
		// Overnight: evening side belongs to today, morning side to the
		// window that started yesterday.
		if minute >= w.start && s.dayAllowed(t.Weekday()) {
			return true
		}
		if minute < w.end && s.dayAllowed(t.Weekday()-1) {
			return true
		}
	}
	return false
}

func (s *Schedule) dayAllowed(day time.Weekday) bool {
	if s.days == nil {
		return true
	}
	if day < 0 {
		day += 7
	}
	return s.days[day]
}

// AllowDispatch reports whether a new agent run may start at now.
func (s *Schedule) AllowDispatch(now time.Time) (bool, string) {
	if !s.activeAt(now) {
		return false, "outside schedule window"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxIssues > 0 && s.issuesStarted >= s.maxIssues {
		return false, fmt.Sprintf("issue budget exhausted (%d/%d)", s.issuesStarted, s.maxIssues)
	}
	if s.maxTokens > 0 && s.tokensUsed >= s.maxTokens {
		return false, fmt.Sprintf("token budget exhausted (%d/%d)", s.tokensUsed, s.maxTokens)
	}
	return true, ""
}

// RecordStart counts a dispatched run against the window's issue budget.
func (s *Schedule) RecordStart(time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.issuesStarted++
}

// RecordCompletion accumulates outcome and token usage for the summary and
// the token budget.
func (s *Schedule) RecordCompletion(succeeded bool, tokens int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if succeeded {
		s.succeeded++
	} else {
		s.failed++
	}
	if tokens > 0 {
		s.tokensUsed += tokens
	}
}

// Tick advances window state. When the schedule transitions from active to
// inactive it returns a human-readable window summary and true; counters
// reset when the next window opens.
func (s *Schedule) Tick(now time.Time) (string, bool) {
	active := s.activeAt(now)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case active && !s.active:
		s.active = true
		s.windowOpenedAt = now
		s.issuesStarted = 0
		s.succeeded = 0
		s.failed = 0
		s.tokensUsed = 0
		s.exhaustedLogged = false
		return "", false
	case !active && s.active:
		s.active = false
		summary := fmt.Sprintf(
			"Schedule window closed (opened %s): %d runs started, %d succeeded, %d failed, %d tokens used",
			s.windowOpenedAt.Format("Mon 15:04"), s.issuesStarted, s.succeeded, s.failed, s.tokensUsed,
		)
		return summary, true
	default:
		return "", false
	}
}

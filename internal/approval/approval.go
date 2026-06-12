// Package approval persists the plan-approval state machine for issues:
// none → planned (plan posted, dispatch parked) → approved (cleared to
// execute). State lives in one JSON file per issue so the CLI, dashboard,
// and a running orchestrator share it without IPC.
package approval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusNone     Status = ""
	StatusPlanned  Status = "planned"
	StatusApproved Status = "approved"
)

// Entry is the persisted approval record for one issue.
type Entry struct {
	IssueID   string    `json:"issue_id"`
	Status    Status    `json:"status"`
	Plan      string    `json:"plan,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	mu  sync.Mutex
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Get returns the issue's approval status and recorded plan; missing files
// mean StatusNone.
func (s *Store) Get(issueID string) (Status, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.read(issueID)
	if err != nil {
		return StatusNone, "", err
	}
	if entry == nil {
		return StatusNone, "", nil
	}
	return entry.Status, entry.Plan, nil
}

// MarkPlanned records the produced plan and parks the issue until approval.
func (s *Store) MarkPlanned(issueID, plan string) error {
	return s.write(Entry{IssueID: issueID, Status: StatusPlanned, Plan: plan})
}

// Approve clears the issue for execution. Approving an unplanned issue is
// allowed (pre-approval skips the planning run entirely).
func (s *Store) Approve(issueID string) error {
	s.mu.Lock()
	existing, err := s.read(issueID)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	entry := Entry{IssueID: issueID, Status: StatusApproved}
	if existing != nil {
		entry.Plan = existing.Plan
	}
	return s.write(entry)
}

// Reset removes the issue's approval state so the next dispatch plans again.
func (s *Store) Reset(issueID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.path(issueID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Pending lists issues parked in planned state, oldest first.
func (s *Store) Pending() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read approvals dir: %w", err)
	}

	var pending []Entry
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		entry, err := s.read(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil || entry == nil {
			continue
		}
		if entry.Status == StatusPlanned {
			pending = append(pending, *entry)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].UpdatedAt.Before(pending[j].UpdatedAt)
	})
	return pending, nil
}

func (s *Store) path(issueID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, issueID)
	return filepath.Join(s.dir, safe+".json")
}

func (s *Store) read(issueID string) (*Entry, error) {
	data, err := os.ReadFile(s.path(issueID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read approval state: %w", err)
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("decode approval state: %w", err)
	}
	return &entry, nil
}

func (s *Store) write(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create approvals dir: %w", err)
	}
	entry.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode approval state: %w", err)
	}
	if err := os.WriteFile(s.path(entry.IssueID), data, 0o644); err != nil {
		return fmt.Errorf("write approval state: %w", err)
	}
	return nil
}

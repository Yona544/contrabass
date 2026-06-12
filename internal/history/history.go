// Package history persists every finished agent run to an append-only JSONL
// file and aggregates it into analytics (success rates, token spend, duration
// percentiles per agent), so the team can see which agent earns which work.
// JSONL keeps the store dependency-free and jq-friendly; the Store interface
// is small enough to move to SQLite later without touching callers.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const runsFileName = "runs.jsonl"

// Record is one finished agent run attempt.
type Record struct {
	IssueID    string    `json:"issue_id"`
	Identifier string    `json:"identifier,omitempty"`
	Title      string    `json:"title,omitempty"`
	AgentType  string    `json:"agent_type,omitempty"`
	Attempt    int       `json:"attempt"`
	Phase      string    `json:"phase"`
	Succeeded  bool      `json:"succeeded"`
	Error      string    `json:"error,omitempty"`
	TokensIn   int64     `json:"tokens_in"`
	TokensOut  int64     `json:"tokens_out"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
}

// AgentStats aggregates outcomes for one agent type.
type AgentStats struct {
	Runs          int   `json:"runs"`
	Succeeded     int   `json:"succeeded"`
	Failed        int   `json:"failed"`
	TokensIn      int64 `json:"tokens_in"`
	TokensOut     int64 `json:"tokens_out"`
	AvgDurationMs int64 `json:"avg_duration_ms"`
}

// Analytics summarizes the whole run history.
type Analytics struct {
	TotalRuns     int                   `json:"total_runs"`
	Succeeded     int                   `json:"succeeded"`
	Failed        int                   `json:"failed"`
	TokensIn      int64                 `json:"tokens_in"`
	TokensOut     int64                 `json:"tokens_out"`
	AvgDurationMs int64                 `json:"avg_duration_ms"`
	ByAgent       map[string]AgentStats `json:"by_agent"`
	GeneratedAt   time.Time             `json:"generated_at"`
}

// Store is a process-safe (single writer) append-only run log.
type Store struct {
	mu   sync.Mutex
	path string
}

func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, runsFileName)}
}

// Append writes one record. Failures are returned for the caller to log;
// history must never fail a run.
func (s *Store) Append(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal history record: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append history record: %w", err)
	}
	return nil
}

// Records returns up to limit records, newest first (0 = all). Corrupt lines
// are skipped so a torn write cannot poison the whole history.
func (s *Store) Records(limit int) ([]Record, error) {
	records, err := s.readAll()
	if err != nil {
		return nil, err
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].FinishedAt.After(records[j].FinishedAt)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

// Analytics aggregates the full history.
func (s *Store) Analytics() (Analytics, error) {
	records, err := s.readAll()
	if err != nil {
		return Analytics{}, err
	}

	analytics := Analytics{
		ByAgent:     make(map[string]AgentStats),
		GeneratedAt: time.Now().UTC(),
	}

	var totalDuration int64
	durations := make(map[string]int64)
	for _, rec := range records {
		analytics.TotalRuns++
		if rec.Succeeded {
			analytics.Succeeded++
		} else {
			analytics.Failed++
		}
		analytics.TokensIn += rec.TokensIn
		analytics.TokensOut += rec.TokensOut
		totalDuration += rec.DurationMs

		agent := rec.AgentType
		if agent == "" {
			agent = "unknown"
		}
		stats := analytics.ByAgent[agent]
		stats.Runs++
		if rec.Succeeded {
			stats.Succeeded++
		} else {
			stats.Failed++
		}
		stats.TokensIn += rec.TokensIn
		stats.TokensOut += rec.TokensOut
		durations[agent] += rec.DurationMs
		analytics.ByAgent[agent] = stats
	}

	if analytics.TotalRuns > 0 {
		analytics.AvgDurationMs = totalDuration / int64(analytics.TotalRuns)
	}
	for agent, stats := range analytics.ByAgent {
		if stats.Runs > 0 {
			stats.AvgDurationMs = durations[agent] / int64(stats.Runs)
		}
		analytics.ByAgent[agent] = stats
	}

	return analytics, nil
}

func (s *Store) readAll() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec Record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read history file: %w", err)
	}
	return records, nil
}

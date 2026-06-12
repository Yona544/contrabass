package main

import (
	"fmt"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/schedule"
)

// buildScheduleGate turns the workflow schedule section into a dispatch gate.
// Returns nil when no schedule is configured.
func buildScheduleGate(cfg *config.WorkflowConfig) (*schedule.Schedule, error) {
	if !cfg.ScheduleEnabled() {
		return nil, nil
	}

	gate, err := schedule.New(schedule.Config{
		Windows:   cfg.Schedule.Windows,
		Days:      cfg.Schedule.Days,
		MaxIssues: cfg.Schedule.MaxIssues,
		MaxTokens: cfg.Schedule.MaxTokens,
		MaxUSD:    cfg.Schedule.MaxUSD,
	})
	if err != nil {
		return nil, fmt.Errorf("parsing schedule config: %w", err)
	}
	return gate, nil
}

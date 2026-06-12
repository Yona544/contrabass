package orchestrator

import (
	"time"

	"github.com/junhoyeo/contrabass/internal/types"
)

type EventType int

const (
	EventStatusUpdate EventType = iota
	EventAgentStarted
	EventAgentFinished
	EventBackoffEnqueued
	EventIssueReleased
	EventScheduleWindowClosed
	EventAgentTranscript
)

func (t EventType) String() string {
	switch t {
	case EventStatusUpdate:
		return "StatusUpdate"
	case EventAgentStarted:
		return "AgentStarted"
	case EventAgentFinished:
		return "AgentFinished"
	case EventBackoffEnqueued:
		return "BackoffEnqueued"
	case EventIssueReleased:
		return "IssueReleased"
	case EventScheduleWindowClosed:
		return "ScheduleWindowClosed"
	case EventAgentTranscript:
		return "AgentTranscript"
	default:
		return "Unknown"
	}
}

type OrchestratorEvent struct {
	Type      EventType
	IssueID   string
	Data      EventPayload
	Timestamp time.Time
}

// EventPayload is a marker interface for typed orchestrator event payloads.
type EventPayload interface {
	eventPayload()
}

type StatusUpdate struct {
	Stats        Stats
	BackoffQueue int
	ModelName    string
	ProjectURL   string
	TrackerType  string
	TrackerScope string
}

func (StatusUpdate) eventPayload() {}

type AgentStarted struct {
	IssueIdentifier string
	IssueURL        string
	Attempt         int
	PID             int
	SessionID       string
	Workspace       string
}

func (AgentStarted) eventPayload() {}

type AgentFinished struct {
	Attempt   int
	Phase     types.RunPhase
	TokensIn  int64
	TokensOut int64
	Error     string
}

func (AgentFinished) eventPayload() {}

type BackoffEnqueued struct {
	Attempt int
	RetryAt time.Time
	Error   string
}

func (BackoffEnqueued) eventPayload() {}

type IssueReleased struct {
	Attempt int
}

func (IssueReleased) eventPayload() {}

// ScheduleWindowClosed reports the end-of-window summary when a dispatch
// schedule is configured.
type ScheduleWindowClosed struct {
	Summary string
}

func (ScheduleWindowClosed) eventPayload() {}

// AgentTranscript carries an assistant message fragment from a running agent
// so UIs can show live transcripts.
type AgentTranscript struct {
	Text string
}

func (AgentTranscript) eventPayload() {}

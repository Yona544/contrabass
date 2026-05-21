package timeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderRunRootComment(t *testing.T) {
	tests := []struct {
		name     string
		run      WorkflowRunSummary
		expected []string
	}{
		{
			name: "uses issue identifier and status",
			run: WorkflowRunSummary{
				IssueID:         "issue-1",
				IssueIdentifier: "ENG-1",
				RunID:           "run-1",
				Attempt:         2,
				Status:          NodeStatusSucceeded,
			},
			expected: []string{
				"Contrabass workflow run ENG-1 (attempt 2) is completed.",
				`<!-- contrabass:workflow-run issue_id="issue-1" run_id="run-1" -->`,
			},
		},
		{
			name: "defaults missing identifier and status",
			run: WorkflowRunSummary{
				IssueID: "issue-2",
				RunID:   "run-2",
				Attempt: 1,
			},
			expected: []string{
				"Contrabass workflow run issue-2 (attempt 1) is started.",
				`<!-- contrabass:workflow-run issue_id="issue-2" run_id="run-2" -->`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := RenderRunRootComment(tt.run)
			for _, expected := range tt.expected {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestRenderRunRootCommentEscapesMarkerValues(t *testing.T) {
	tests := []struct {
		name          string
		run           WorkflowRunSummary
		wantAttribute string
		rawAttribute  string
	}{
		{
			name: "separates double hyphen in issue id",
			run: WorkflowRunSummary{
				IssueID: "issue--1",
				RunID:   "run-1",
				Attempt: 1,
			},
			wantAttribute: `issue_id="issue- -1"`,
			rawAttribute:  `issue_id="issue--1"`,
		},
		{
			name: "escapes mixed run id",
			run: WorkflowRunSummary{
				IssueID: "issue-1",
				RunID:   `run"--\1`,
				Attempt: 1,
			},
			wantAttribute: `run_id="run\"- -\\1"`,
			rawAttribute:  `run_id="run\"--\\1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := RenderRunRootComment(tt.run)

			assert.Contains(t, body, tt.wantAttribute)
			assert.NotContains(t, body, tt.rawAttribute)
			assert.Contains(t, body, "<!-- contrabass:workflow-run ")
			assert.Contains(t, body, " -->")
		})
	}
}

func TestRenderNodeComment(t *testing.T) {
	tests := []struct {
		name     string
		node     WorkflowNodeSummary
		expected []string
	}{
		{
			name: "uses title and optional fields",
			node: WorkflowNodeSummary{
				IssueID:     "issue-1",
				RunID:       "run-1",
				NodeID:      "node-1",
				Attempt:     2,
				Status:      NodeStatusFailed,
				Title:       "Run tests",
				Summary:     "go test failed",
				Error:       "exit status 1",
				TokensIn:    10,
				TokensOut:   20,
				ContentHash: "hash-1",
			},
			expected: []string{
				"Contrabass workflow node: Run tests",
				"Status: failed (attempt 2)",
				"go test failed",
				"Error: exit status 1",
				"Tokens: in=10 out=20",
				`<!-- contrabass:workflow-node issue_id="issue-1" run_id="run-1" node_id="node-1" content_hash="hash-1" -->`,
			},
		},
		{
			name: "defaults missing title to node id",
			node: WorkflowNodeSummary{
				IssueID:     "issue-2",
				RunID:       "run-2",
				NodeID:      "node-2",
				Status:      NodeStatusStarted,
				ContentHash: "hash-2",
			},
			expected: []string{
				"Contrabass workflow node: node-2",
				"Status: started",
				`<!-- contrabass:workflow-node issue_id="issue-2" run_id="run-2" node_id="node-2" content_hash="hash-2" -->`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := RenderNodeComment(tt.node)
			for _, expected := range tt.expected {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestRenderNodeCommentEscapesMarkerValues(t *testing.T) {
	tests := []struct {
		name          string
		node          WorkflowNodeSummary
		wantAttribute string
		rawAttribute  string
	}{
		{
			name: "separates double hyphen in issue id",
			node: WorkflowNodeSummary{
				IssueID:     "issue--1",
				RunID:       "run-1",
				NodeID:      "node-1",
				Status:      NodeStatusSucceeded,
				ContentHash: "hash-1",
			},
			wantAttribute: `issue_id="issue- -1"`,
			rawAttribute:  `issue_id="issue--1"`,
		},
		{
			name: "escapes mixed node id",
			node: WorkflowNodeSummary{
				IssueID:     "issue-1",
				RunID:       "run-1",
				NodeID:      `node"--\1`,
				Status:      NodeStatusSucceeded,
				ContentHash: "hash-1",
			},
			wantAttribute: `node_id="node\"- -\\1"`,
			rawAttribute:  `node_id="node\"--\\1"`,
		},
		{
			name: "escapes mixed content hash",
			node: WorkflowNodeSummary{
				IssueID:     "issue-1",
				RunID:       "run-1",
				NodeID:      "node-1",
				Status:      NodeStatusSucceeded,
				ContentHash: `hash"--\1`,
			},
			wantAttribute: `content_hash="hash\"- -\\1"`,
			rawAttribute:  `content_hash="hash\"--\\1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := RenderNodeComment(tt.node)

			assert.Contains(t, body, tt.wantAttribute)
			assert.NotContains(t, body, tt.rawAttribute)
			assert.Contains(t, body, "<!-- contrabass:workflow-node ")
			assert.Contains(t, body, " -->")
		})
	}
}

package tracker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJiraDefaultJQL = `project = "PROJ" AND statusCategory != Done ORDER BY priority DESC, created ASC`

func testJiraConfig(url string) JiraConfig {
	return JiraConfig{
		BaseURL:   url,
		Email:     "bot@example.com",
		APIToken:  "jira_test_token",
		Project:   "PROJ",
		AccountID: "5b10ac8d82e05b22cc7d4ef5",
		PageSize:  50,
	}
}

func testJiraClient(t *testing.T, url string) *JiraClient {
	t.Helper()
	client, err := NewJiraClient(testJiraConfig(url))
	require.NoError(t, err)
	return client
}

func respondJiraJSON(w http.ResponseWriter, statusCode int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func jiraTransitionsBody() map[string]interface{} {
	return map[string]interface{}{
		"transitions": []interface{}{
			map[string]interface{}{
				"id":   "11",
				"name": "To Do",
				"to": map[string]interface{}{
					"name":           "To Do",
					"statusCategory": map[string]interface{}{"key": "new"},
				},
			},
			map[string]interface{}{
				"id":   "21",
				"name": "Start Progress",
				"to": map[string]interface{}{
					"name":           "In Progress",
					"statusCategory": map[string]interface{}{"key": "indeterminate"},
				},
			},
			map[string]interface{}{
				"id":   "31",
				"name": "Done",
				"to": map[string]interface{}{
					"name":           "Done",
					"statusCategory": map[string]interface{}{"key": "done"},
				},
			},
			map[string]interface{}{
				"id":   "41",
				"name": "Ship It",
				"to": map[string]interface{}{
					"name":           "Shipped",
					"statusCategory": map[string]interface{}{"key": "done"},
				},
			},
		},
	}
}

func TestNewJiraClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*JiraConfig)
		wantErr string
	}{
		{name: "valid config", mutate: func(cfg *JiraConfig) {}},
		{
			name:    "missing base url",
			mutate:  func(cfg *JiraConfig) { cfg.BaseURL = "" },
			wantErr: "jira base url required",
		},
		{
			name:    "missing email",
			mutate:  func(cfg *JiraConfig) { cfg.Email = "" },
			wantErr: "jira email required",
		},
		{
			name:    "missing api token",
			mutate:  func(cfg *JiraConfig) { cfg.APIToken = "  " },
			wantErr: "jira api token required",
		},
		{
			name: "missing project and jql",
			mutate: func(cfg *JiraConfig) {
				cfg.Project = ""
				cfg.JQL = ""
			},
			wantErr: "jira project or jql required",
		},
		{
			name: "jql without project is valid",
			mutate: func(cfg *JiraConfig) {
				cfg.Project = ""
				cfg.JQL = "assignee = currentUser()"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testJiraConfig("https://example.atlassian.net")
			tt.mutate(&cfg)

			client, err := NewJiraClient(cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}

func TestJiraSearchJQL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*JiraConfig)
		want   string
	}{
		{
			name:   "default jql",
			mutate: func(cfg *JiraConfig) {},
			want:   testJiraDefaultJQL,
		},
		{
			name:   "default jql with labels",
			mutate: func(cfg *JiraConfig) { cfg.Labels = []string{"backend", "p0"} },
			want:   `project = "PROJ" AND statusCategory != Done AND labels IN ("backend", "p0") ORDER BY priority DESC, created ASC`,
		},
		{
			name:   "explicit jql wins",
			mutate: func(cfg *JiraConfig) { cfg.JQL = "assignee = currentUser()" },
			want:   "assignee = currentUser()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testJiraConfig("https://example.atlassian.net")
			tt.mutate(&cfg)

			client, err := NewJiraClient(cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, client.searchJQL())
		})
	}
}

func TestJiraFetchIssues_ParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/rest/api/2/search", r.URL.Path)
		assert.Equal(t, testJiraDefaultJQL, r.URL.Query().Get("jql"))
		assert.Equal(t, "0", r.URL.Query().Get("startAt"))
		assert.Equal(t, "50", r.URL.Query().Get("maxResults"))
		assert.Equal(t, jiraSearchFields, r.URL.Query().Get("fields"))

		respondJiraJSON(w, http.StatusOK, map[string]interface{}{
			"startAt":    0,
			"maxResults": 50,
			"total":      2,
			"issues": []interface{}{
				map[string]interface{}{
					"id":  "10042",
					"key": "PROJ-42",
					"fields": map[string]interface{}{
						"summary":     "Fix parser panic",
						"description": "Parser crashes on malformed JSON",
						"status": map[string]interface{}{
							"name":           "To Do",
							"statusCategory": map[string]interface{}{"key": "new", "name": "To Do"},
						},
						"priority": map[string]interface{}{"id": "2", "name": "High"},
						"labels":   []interface{}{"Bug", "P0"},
						"created":  "2026-03-01T10:30:00.000+0000",
						"updated":  "2026-03-02T15:45:00.000+0000",
						"issuelinks": []interface{}{
							map[string]interface{}{
								"type":        map[string]interface{}{"name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
								"inwardIssue": map[string]interface{}{"key": "PROJ-40"},
							},
							map[string]interface{}{
								"type":         map[string]interface{}{"name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
								"outwardIssue": map[string]interface{}{"key": "PROJ-50"},
							},
							map[string]interface{}{
								"type":        map[string]interface{}{"name": "Relates", "inward": "relates to", "outward": "relates to"},
								"inwardIssue": map[string]interface{}{"key": "PROJ-41"},
							},
						},
					},
				},
				map[string]interface{}{
					"id":  "10043",
					"key": "PROJ-43",
					"fields": map[string]interface{}{
						"summary":     "Improve logs",
						"description": "Add more context",
						"status": map[string]interface{}{
							"name":           "In Progress",
							"statusCategory": map[string]interface{}{"key": "indeterminate", "name": "In Progress"},
						},
						"labels": []interface{}{"Enhancement"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	issues, err := client.FetchIssues(context.Background())

	require.NoError(t, err)
	require.Len(t, issues, 2)

	assert.Equal(t, "PROJ-42", issues[0].ID)
	assert.Equal(t, "PROJ-42", issues[0].Identifier)
	assert.Equal(t, "Fix parser panic", issues[0].Title)
	assert.Equal(t, "Parser crashes on malformed JSON", issues[0].Description)
	assert.Equal(t, types.Unclaimed, issues[0].State)
	assert.Equal(t, 2, issues[0].Priority)
	assert.Equal(t, []string{"bug", "p0"}, issues[0].Labels)
	assert.Equal(t, server.URL+"/browse/PROJ-42", issues[0].URL)
	assert.Equal(t, "symphony/proj-42", issues[0].BranchName)
	assert.Equal(t, []string{"PROJ-40"}, issues[0].BlockedBy)
	assert.Equal(t, "2026-03-01T10:30:00Z", issues[0].CreatedAt.UTC().Format(time.RFC3339))
	assert.Equal(t, "2026-03-02T15:45:00Z", issues[0].UpdatedAt.UTC().Format(time.RFC3339))
	assert.Equal(t, "10042", issues[0].TrackerMeta["jira_id"])
	assert.Equal(t, "To Do", issues[0].TrackerMeta["jira_status"])
	assert.Equal(t, "new", issues[0].TrackerMeta["jira_status_category"])

	assert.Equal(t, "PROJ-43", issues[1].ID)
	assert.Equal(t, types.Claimed, issues[1].State)
	assert.Equal(t, 0, issues[1].Priority)
	assert.Equal(t, []string{"enhancement"}, issues[1].Labels)
	assert.Equal(t, []string{}, issues[1].BlockedBy)
}

func TestJiraFetchIssues_FiltersDoneCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJiraJSON(w, http.StatusOK, map[string]interface{}{
			"total": 2,
			"issues": []interface{}{
				map[string]interface{}{
					"key": "PROJ-1",
					"fields": map[string]interface{}{
						"summary": "Open issue",
						"status": map[string]interface{}{
							"name":           "To Do",
							"statusCategory": map[string]interface{}{"key": "new"},
						},
					},
				},
				map[string]interface{}{
					"key": "PROJ-2",
					"fields": map[string]interface{}{
						"summary": "Finished issue",
						"status": map[string]interface{}{
							"name":           "Done",
							"statusCategory": map[string]interface{}{"key": "done"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	issues, err := client.FetchIssues(context.Background())

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "PROJ-1", issues[0].ID)
}

func TestJiraFetchIssues_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJiraJSON(w, http.StatusOK, map[string]interface{}{
			"total":  0,
			"issues": []interface{}{},
		})
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	issues, err := client.FetchIssues(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, issues)
	assert.Equal(t, []types.Issue{}, issues)
}

func TestJiraFetchIssues_Pagination(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startAt := r.URL.Query().Get("startAt")
		switch requestCount.Add(1) {
		case 1:
			assert.Equal(t, "0", startAt)
			respondJiraJSON(w, http.StatusOK, map[string]interface{}{
				"startAt": 0,
				"total":   3,
				"issues": []interface{}{
					map[string]interface{}{"key": "PROJ-10", "fields": map[string]interface{}{"summary": "Issue 10"}},
					map[string]interface{}{"key": "PROJ-11", "fields": map[string]interface{}{"summary": "Issue 11"}},
				},
			})
		case 2:
			assert.Equal(t, "2", startAt)
			respondJiraJSON(w, http.StatusOK, map[string]interface{}{
				"startAt": 2,
				"total":   3,
				"issues": []interface{}{
					map[string]interface{}{"key": "PROJ-12", "fields": map[string]interface{}{"summary": "Issue 12"}},
				},
			})
		default:
			t.Fatalf("unexpected extra request")
		}
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	client.pageSize = 2
	issues, err := client.FetchIssues(context.Background())

	require.NoError(t, err)
	assert.Len(t, issues, 3)
	assert.Equal(t, int32(2), requestCount.Load())
}

func TestJiraFetchIssues_HTTPErrors(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		headers     map[string]string
		body        interface{}
		wantRate    bool
		wantAuth    bool
		wantGeneric bool
	}{
		{
			name:       "429 returns RateLimitError",
			statusCode: http.StatusTooManyRequests,
			headers:    map[string]string{"Retry-After": "15"},
			body:       map[string]interface{}{"error": "rate limited"},
			wantRate:   true,
		},
		{
			name:       "401 returns AuthError",
			statusCode: http.StatusUnauthorized,
			body:       map[string]interface{}{"error": "unauthorized"},
			wantAuth:   true,
		},
		{
			name:        "500 returns generic error",
			statusCode:  http.StatusInternalServerError,
			body:        map[string]interface{}{"error": "boom"},
			wantGeneric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				respondJiraJSON(w, tt.statusCode, tt.body)
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			_, err := client.FetchIssues(context.Background())

			require.Error(t, err)

			if tt.wantRate {
				var rlErr *RateLimitError
				assert.ErrorAs(t, err, &rlErr)
				assert.Equal(t, 15*time.Second, rlErr.RetryAfter)
			}
			if tt.wantAuth {
				var authErr *AuthError
				assert.ErrorAs(t, err, &authErr)
				assert.Equal(t, http.StatusUnauthorized, authErr.StatusCode)
			}
			if tt.wantGeneric {
				assert.Contains(t, err.Error(), "jira API error")
			}
		})
	}
}

func TestJiraFetchIssues_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	_, err := client.FetchIssues(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing Jira search response")
}

func TestJiraClaimIssue(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "happy path", statusCode: http.StatusNoContent, wantErr: false},
		{name: "wrong success status", statusCode: http.StatusOK, wantErr: true},
		{name: "http error", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/rest/api/2/issue/PROJ-42/assignee", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body := parseJSONBody(t, r)
				assert.Equal(t, "5b10ac8d82e05b22cc7d4ef5", body["accountId"])

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			err := client.ClaimIssue(context.Background(), "PROJ-42")

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJiraReleaseIssue(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "happy path", statusCode: http.StatusNoContent, wantErr: false},
		{name: "wrong success status", statusCode: http.StatusOK, wantErr: true},
		{name: "http error", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/rest/api/2/issue/PROJ-42/assignee", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body := parseJSONBody(t, r)
				accountID, present := body["accountId"]
				require.True(t, present)
				assert.Nil(t, accountID)

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			err := client.ReleaseIssue(context.Background(), "PROJ-42")

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJiraUpdateIssueState_ResolvesTransitionDynamically(t *testing.T) {
	tests := []struct {
		name             string
		state            types.IssueState
		wantTransitionID string
	}{
		{name: "Claimed starts progress", state: types.Claimed, wantTransitionID: "21"},
		{name: "Running starts progress", state: types.Running, wantTransitionID: "21"},
		{name: "Released moves to done", state: types.Released, wantTransitionID: "31"},
		{name: "Unclaimed moves back to to-do", state: types.Unclaimed, wantTransitionID: "11"},
		{name: "RetryQueued moves back to to-do", state: types.RetryQueued, wantTransitionID: "11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotTransitionID atomic.Value
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/rest/api/2/issue/PROJ-42/transitions", r.URL.Path)
				switch r.Method {
				case http.MethodGet:
					respondJiraJSON(w, http.StatusOK, jiraTransitionsBody())
				case http.MethodPost:
					body := parseJSONBody(t, r)
					transition, ok := body["transition"].(map[string]interface{})
					require.True(t, ok)
					gotTransitionID.Store(transition["id"])
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Fatalf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			err := client.UpdateIssueState(context.Background(), "PROJ-42", tt.state)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTransitionID, gotTransitionID.Load())
		})
	}
}

func TestJiraUpdateIssueState_ExplicitOverride(t *testing.T) {
	tests := []struct {
		name             string
		mutate           func(*JiraConfig)
		state            types.IssueState
		wantTransitionID string
	}{
		{
			name:             "override by name beats category match order",
			mutate:           func(cfg *JiraConfig) { cfg.TransitionDone = "Ship It" },
			state:            types.Released,
			wantTransitionID: "41",
		},
		{
			name:             "override by id",
			mutate:           func(cfg *JiraConfig) { cfg.TransitionInProgress = "21" },
			state:            types.Running,
			wantTransitionID: "21",
		},
		{
			name:             "failed override applies to retry queue",
			mutate:           func(cfg *JiraConfig) { cfg.TransitionFailed = "To Do" },
			state:            types.RetryQueued,
			wantTransitionID: "11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotTransitionID atomic.Value
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					respondJiraJSON(w, http.StatusOK, jiraTransitionsBody())
				case http.MethodPost:
					body := parseJSONBody(t, r)
					transition, ok := body["transition"].(map[string]interface{})
					require.True(t, ok)
					gotTransitionID.Store(transition["id"])
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()

			cfg := testJiraConfig(server.URL)
			tt.mutate(&cfg)
			client, err := NewJiraClient(cfg)
			require.NoError(t, err)

			err = client.UpdateIssueState(context.Background(), "PROJ-42", tt.state)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTransitionID, gotTransitionID.Load())
		})
	}
}

func TestJiraUpdateIssueState_NoMatchingTransition(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*JiraConfig)
		state   types.IssueState
		wantErr string
	}{
		{
			name:    "no transition to target category",
			mutate:  func(cfg *JiraConfig) {},
			state:   types.Released,
			wantErr: `no transition to status category "done" available`,
		},
		{
			name:    "override not available",
			mutate:  func(cfg *JiraConfig) { cfg.TransitionInProgress = "Reopen" },
			state:   types.Claimed,
			wantErr: `transition "Reopen" not available`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				respondJiraJSON(w, http.StatusOK, map[string]interface{}{
					"transitions": []interface{}{
						map[string]interface{}{
							"id":   "11",
							"name": "To Do",
							"to": map[string]interface{}{
								"name":           "To Do",
								"statusCategory": map[string]interface{}{"key": "new"},
							},
						},
					},
				})
			}))
			defer server.Close()

			cfg := testJiraConfig(server.URL)
			tt.mutate(&cfg)
			client, err := NewJiraClient(cfg)
			require.NoError(t, err)

			err = client.UpdateIssueState(context.Background(), "PROJ-42", tt.state)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "resolving transition for issue PROJ-42")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestJiraUpdateIssueState_TransitionPostFails(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "wrong success status", statusCode: http.StatusOK},
		{name: "http error", statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					respondJiraJSON(w, http.StatusOK, jiraTransitionsBody())
				case http.MethodPost:
					respondJiraJSON(w, tt.statusCode, map[string]interface{}{"error": "boom"})
				}
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			err := client.UpdateIssueState(context.Background(), "PROJ-42", types.Released)

			require.Error(t, err)
		})
	}
}

func TestJiraUpdateIssueState_TransitionsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	err := client.UpdateIssueState(context.Background(), "PROJ-42", types.Released)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching transitions for issue PROJ-42")
	assert.Contains(t, err.Error(), "parsing Jira transitions response")
}

func TestJiraPostComment(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "happy path", statusCode: http.StatusCreated, wantErr: false},
		{name: "wrong success status", statusCode: http.StatusOK, wantErr: true},
		{name: "http error", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/rest/api/2/issue/PROJ-42/comment", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body := parseJSONBody(t, r)
				assert.Equal(t, "hello from contrabass", body["body"])

				respondJiraJSON(w, tt.statusCode, map[string]interface{}{"id": "10000"})
			}))
			defer server.Close()

			client := testJiraClient(t, server.URL)
			err := client.PostComment(context.Background(), "PROJ-42", "hello from contrabass")

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJiraClient_AuthHeaders(t *testing.T) {
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("bot@example.com:jira_test_token"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, wantAuth, r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		respondJiraJSON(w, http.StatusOK, map[string]interface{}{"total": 0, "issues": []interface{}{}})
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL)
	_, err := client.FetchIssues(context.Background())
	require.NoError(t, err)
}

func TestJiraClient_BaseURLTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rest/api/2/search", r.URL.Path)
		respondJiraJSON(w, http.StatusOK, map[string]interface{}{"total": 0, "issues": []interface{}{}})
	}))
	defer server.Close()

	client := testJiraClient(t, server.URL+"/")
	_, err := client.FetchIssues(context.Background())
	require.NoError(t, err)
}

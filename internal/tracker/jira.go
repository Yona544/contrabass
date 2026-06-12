package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/junhoyeo/contrabass/internal/types"
)

// jiraSearchFields lists the issue fields requested from the Jira search API.
const jiraSearchFields = "summary,description,status,priority,labels,issuelinks,created,updated"

// jiraTimeLayout matches Jira Cloud timestamps like "2026-03-01T10:30:00.000+0000".
const jiraTimeLayout = "2006-01-02T15:04:05.000-0700"

// Jira Cloud status category keys. Every Jira status belongs to exactly one of
// these three categories, regardless of how workflows name their statuses.
const (
	jiraCategoryToDo       = "new"
	jiraCategoryInProgress = "indeterminate"
	jiraCategoryDone       = "done"
)

// JiraConfig configures the Jira Cloud REST client.
type JiraConfig struct {
	BaseURL              string // e.g. https://yourcompany.atlassian.net
	Email                string // Atlassian account email for basic auth
	APIToken             string // Atlassian API token for basic auth
	Project              string // project key used to build the default JQL
	JQL                  string // overrides the default JQL when set
	AccountID            string // Atlassian account ID for claiming issues
	Labels               []string
	TransitionInProgress string // explicit transition name or ID override
	TransitionDone       string // explicit transition name or ID override
	TransitionFailed     string // explicit transition name or ID override
	HTTPClient           *http.Client
	PageSize             int
}

// JiraClient implements the Tracker interface using the Jira Cloud REST API v2.
type JiraClient struct {
	baseURL              string
	email                string
	apiToken             string
	project              string
	jql                  string
	accountID            string
	labels               []string
	transitionInProgress string
	transitionDone       string
	transitionFailed     string
	httpClient           *http.Client
	pageSize             int
}

// Compile-time interface satisfaction check.
var _ Tracker = (*JiraClient)(nil)

// NewJiraClient creates a new JiraClient with the given configuration.
func NewJiraClient(cfg JiraConfig) (*JiraClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("jira base url required")
	}
	if strings.TrimSpace(cfg.Email) == "" {
		return nil, errors.New("jira email required")
	}
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, errors.New("jira api token required")
	}
	if strings.TrimSpace(cfg.Project) == "" && strings.TrimSpace(cfg.JQL) == "" {
		return nil, errors.New("jira project or jql required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}

	return &JiraClient{
		baseURL:              strings.TrimSuffix(cfg.BaseURL, "/"),
		email:                cfg.Email,
		apiToken:             cfg.APIToken,
		project:              cfg.Project,
		jql:                  cfg.JQL,
		accountID:            cfg.AccountID,
		labels:               cfg.Labels,
		transitionInProgress: cfg.TransitionInProgress,
		transitionDone:       cfg.TransitionDone,
		transitionFailed:     cfg.TransitionFailed,
		httpClient:           httpClient,
		pageSize:             pageSize,
	}, nil
}

type jiraSearchResponse struct {
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
	Issues     []jiraIssue `json:"issues"`
}

type jiraIssue struct {
	ID     string     `json:"id"`
	Key    string     `json:"key"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Status      jiraStatus      `json:"status"`
	Priority    *jiraPriority   `json:"priority"`
	Labels      []string        `json:"labels"`
	IssueLinks  []jiraIssueLink `json:"issuelinks"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
}

type jiraStatus struct {
	Name           string             `json:"name"`
	StatusCategory jiraStatusCategory `json:"statusCategory"`
}

type jiraStatusCategory struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type jiraPriority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type jiraIssueLink struct {
	Type        jiraIssueLinkType `json:"type"`
	InwardIssue *jiraLinkedIssue  `json:"inwardIssue"`
}

type jiraIssueLinkType struct {
	Name   string `json:"name"`
	Inward string `json:"inward"`
}

type jiraLinkedIssue struct {
	Key string `json:"key"`
}

type jiraTransitionsResponse struct {
	Transitions []jiraTransition `json:"transitions"`
}

type jiraTransition struct {
	ID   string               `json:"id"`
	Name string               `json:"name"`
	To   jiraTransitionStatus `json:"to"`
}

type jiraTransitionStatus struct {
	Name           string             `json:"name"`
	StatusCategory jiraStatusCategory `json:"statusCategory"`
}

// FetchIssues retrieves candidate issues from Jira with startAt-based pagination.
func (c *JiraClient) FetchIssues(ctx context.Context) ([]types.Issue, error) {
	issues := make([]types.Issue, 0)

	for startAt := 0; ; {
		values := url.Values{}
		values.Set("jql", c.searchJQL())
		values.Set("startAt", strconv.Itoa(startAt))
		values.Set("maxResults", strconv.Itoa(c.pageSize))
		values.Set("fields", jiraSearchFields)

		path := "/rest/api/2/search?" + values.Encode()
		body, _, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		var page jiraSearchResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing Jira search response: %w", err)
		}

		for _, item := range page.Issues {
			if !isClaimableJiraStatusCategory(item.Fields.Status.StatusCategory.Key) {
				continue
			}
			issues = append(issues, c.normalizeIssue(item))
		}

		startAt += len(page.Issues)
		if len(page.Issues) == 0 || startAt >= page.Total {
			break
		}
	}

	return issues, nil
}

// ClaimIssue assigns the issue to the configured Atlassian account.
func (c *JiraClient) ClaimIssue(ctx context.Context, issueID string) error {
	payload := map[string]interface{}{
		"accountId": c.accountID,
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/assignee", issueID)
	_, _, statusCode, err := c.doRequestWithStatus(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}

	if statusCode != http.StatusNoContent {
		return fmt.Errorf("jira API error: expected status 204, got %d", statusCode)
	}

	return nil
}

// ReleaseIssue removes the assignee from the issue.
func (c *JiraClient) ReleaseIssue(ctx context.Context, issueID string) error {
	payload := map[string]interface{}{
		"accountId": nil,
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/assignee", issueID)
	_, _, statusCode, err := c.doRequestWithStatus(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}

	if statusCode != http.StatusNoContent {
		return fmt.Errorf("jira API error: expected status 204, got %d", statusCode)
	}

	return nil
}

// UpdateIssueState transitions the issue toward the status category mapped from
// the internal state. Jira workflows are user-defined, so the transition is
// resolved dynamically from the issue's available transitions; an explicit
// transition name or ID configured in JiraConfig takes precedence.
func (c *JiraClient) UpdateIssueState(ctx context.Context, issueID string, state types.IssueState) error {
	targetCategory, override := c.transitionTarget(state)

	transitions, err := c.fetchTransitions(ctx, issueID)
	if err != nil {
		return fmt.Errorf("fetching transitions for issue %s: %w", issueID, err)
	}

	transitionID, err := resolveJiraTransition(transitions, targetCategory, override)
	if err != nil {
		return fmt.Errorf("resolving transition for issue %s: %w", issueID, err)
	}

	payload := map[string]interface{}{
		"transition": map[string]string{"id": transitionID},
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueID)
	_, _, statusCode, err := c.doRequestWithStatus(ctx, http.MethodPost, path, payload)
	if err != nil {
		return fmt.Errorf("transitioning issue %s: %w", issueID, err)
	}

	if statusCode != http.StatusNoContent {
		return fmt.Errorf("jira API error: expected status 204, got %d", statusCode)
	}

	return nil
}

// PostComment creates a plain-text comment on the issue.
func (c *JiraClient) PostComment(ctx context.Context, issueID string, body string) error {
	payload := map[string]string{
		"body": body,
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/comment", issueID)
	_, _, statusCode, err := c.doRequestWithStatus(ctx, http.MethodPost, path, payload)
	if err != nil {
		return err
	}

	if statusCode != http.StatusCreated {
		return fmt.Errorf("jira API error: expected status 201, got %d", statusCode)
	}

	return nil
}

// searchJQL returns the configured JQL, or builds the default query scoped to
// the project, excluding Done-category issues and honoring label filters.
func (c *JiraClient) searchJQL() string {
	if c.jql != "" {
		return c.jql
	}

	jql := fmt.Sprintf("project = %q AND statusCategory != Done", c.project)
	if len(c.labels) > 0 {
		quoted := make([]string, 0, len(c.labels))
		for _, label := range c.labels {
			quoted = append(quoted, strconv.Quote(label))
		}
		jql += fmt.Sprintf(" AND labels IN (%s)", strings.Join(quoted, ", "))
	}

	return jql + " ORDER BY priority DESC, created ASC"
}

// transitionTarget maps an internal issue state to the desired Jira status
// category and the explicit transition override configured for it, if any.
func (c *JiraClient) transitionTarget(state types.IssueState) (string, string) {
	switch state {
	case types.Claimed, types.Running:
		return jiraCategoryInProgress, c.transitionInProgress
	case types.Released:
		return jiraCategoryDone, c.transitionDone
	case types.Unclaimed, types.RetryQueued:
		return jiraCategoryToDo, c.transitionFailed
	default:
		return jiraCategoryToDo, c.transitionFailed
	}
}

// fetchTransitions retrieves the transitions currently available for the issue.
func (c *JiraClient) fetchTransitions(ctx context.Context, issueID string) ([]jiraTransition, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", issueID)
	body, _, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var result jiraTransitionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing Jira transitions response: %w", err)
	}

	return result.Transitions, nil
}

// resolveJiraTransition picks the transition to execute. An explicit override
// matches by transition ID or case-insensitive name; otherwise the first
// transition targeting the desired status category wins.
func resolveJiraTransition(transitions []jiraTransition, targetCategory string, override string) (string, error) {
	if override != "" {
		for _, tr := range transitions {
			if tr.ID == override || strings.EqualFold(tr.Name, override) {
				return tr.ID, nil
			}
		}
		return "", fmt.Errorf("transition %q not available", override)
	}

	for _, tr := range transitions {
		if strings.EqualFold(tr.To.StatusCategory.Key, targetCategory) {
			return tr.ID, nil
		}
	}

	return "", fmt.Errorf("no transition to status category %q available", targetCategory)
}

func (c *JiraClient) doRequest(ctx context.Context, method string, path string, body interface{}) ([]byte, http.Header, error) {
	respBody, headers, _, err := c.doRequestWithStatus(ctx, method, path, body)
	return respBody, headers, err
}

func (c *JiraClient) doRequestWithStatus(ctx context.Context, method string, path string, body interface{}) ([]byte, http.Header, int, error) {
	fullURL := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("creating request: %w", err)
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("reading response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, resp.Header, resp.StatusCode, &RateLimitError{RetryAfter: retryAfter}
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, resp.Header, resp.StatusCode, &AuthError{StatusCode: resp.StatusCode, Message: string(respBody)}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, truncateBody(respBody))
	}

	return respBody, resp.Header, resp.StatusCode, nil
}

func (c *JiraClient) normalizeIssue(item jiraIssue) types.Issue {
	labels := make([]string, 0, len(item.Fields.Labels))
	for _, label := range item.Fields.Labels {
		if label == "" {
			continue
		}
		labels = append(labels, strings.ToLower(label))
	}

	createdAt := parseJiraTime(item.Key, "created", item.Fields.Created)
	updatedAt := parseJiraTime(item.Key, "updated", item.Fields.Updated)

	priority := 0
	if item.Fields.Priority != nil {
		if parsed, err := strconv.Atoi(item.Fields.Priority.ID); err == nil {
			priority = parsed
		}
	}

	statusCategory := item.Fields.Status.StatusCategory.Key

	return types.Issue{
		ID:            item.Key,
		Identifier:    item.Key,
		Title:         item.Fields.Summary,
		Description:   item.Fields.Description,
		State:         issueStateFromJiraStatusCategory(statusCategory),
		Priority:      priority,
		Labels:        labels,
		URL:           c.baseURL + "/browse/" + item.Key,
		BranchName:    "symphony/" + strings.ToLower(item.Key),
		BlockedBy:     extractJiraBlockedBy(item.Fields.IssueLinks),
		ModelOverride: ParseModelOverride(item.Fields.Description),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		TrackerMeta: map[string]interface{}{
			"jira_id":              item.ID,
			"jira_status":          item.Fields.Status.Name,
			"jira_status_category": statusCategory,
		},
	}
}

// isClaimableJiraStatusCategory reports whether an issue with this status
// category should be returned to the orchestrator. The Done category is
// excluded so a just-released issue does not reappear as Unclaimed in the next
// poll when a custom JQL forgets to filter it. An empty string is treated as
// claimable, mirroring the Linear client's handling of missing state types.
func isClaimableJiraStatusCategory(key string) bool {
	return !strings.EqualFold(strings.TrimSpace(key), jiraCategoryDone)
}

// issueStateFromJiraStatusCategory preserves Jira's active status category so
// an issue already in progress does not look freshly unclaimed after a restart.
func issueStateFromJiraStatusCategory(key string) types.IssueState {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", jiraCategoryToDo:
		return types.Unclaimed
	case jiraCategoryInProgress:
		return types.Claimed
	default:
		return types.Claimed
	}
}

// extractJiraBlockedBy returns the keys of issues blocking this one. A Jira
// "Blocks" link with an inwardIssue reads as "inwardIssue blocks this issue",
// so the blockers are the inwardIssue keys. Missing or malformed links are
// skipped; the result is always a non-nil slice.
func extractJiraBlockedBy(links []jiraIssueLink) []string {
	blockers := make([]string, 0, len(links))
	for _, link := range links {
		if !strings.EqualFold(link.Type.Name, "Blocks") {
			continue
		}
		if link.InwardIssue == nil || link.InwardIssue.Key == "" {
			continue
		}
		blockers = append(blockers, link.InwardIssue.Key)
	}
	return blockers
}

// parseJiraTime parses a Jira Cloud timestamp, accepting both Jira's legacy
// "+0000" zone format and RFC 3339.
func parseJiraTime(issueKey string, field string, value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	for _, layout := range []string{jiraTimeLayout, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}

	slog.Debug("failed to parse jira issue timestamp", "issue_key", issueKey, "field", field, "value", value)
	return time.Time{}
}

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/approval"
	"github.com/junhoyeo/contrabass/internal/config"
)

func runApproveCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := newApproveCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestApproveParkedPlanPrintsPlan(t *testing.T) {
	dir := t.TempDir()
	store := approval.NewStore(dir)
	require.NoError(t, store.MarkPlanned("CB-1", "1. fix it\n2. test it"))

	stdout, err := runApproveCmd(t, "CB-1", "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "approved CB-1")
	assert.Contains(t, stdout, "1. fix it")

	status, _, err := store.Get("CB-1")
	require.NoError(t, err)
	assert.Equal(t, approval.StatusApproved, status)
}

func TestApprovePreApprovesUnplannedIssue(t *testing.T) {
	dir := t.TempDir()

	stdout, err := runApproveCmd(t, "CB-2", "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "pre-approved CB-2")
}

func TestApproveListShowsPending(t *testing.T) {
	dir := t.TempDir()
	store := approval.NewStore(dir)
	require.NoError(t, store.MarkPlanned("CB-3", "the plan headline\nmore detail"))

	stdout, err := runApproveCmd(t, "--list", "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "CB-3")
	assert.Contains(t, stdout, "the plan headline")

	empty := t.TempDir()
	stdout, err = runApproveCmd(t, "--list", "--dir", empty)
	require.NoError(t, err)
	assert.Contains(t, stdout, "no plans awaiting approval")
}

func TestApproveResetClearsState(t *testing.T) {
	dir := t.TempDir()
	store := approval.NewStore(dir)
	require.NoError(t, store.MarkPlanned("CB-4", "plan"))

	stdout, err := runApproveCmd(t, "CB-4", "--reset", "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "cleared")

	status, _, err := store.Get("CB-4")
	require.NoError(t, err)
	assert.Equal(t, approval.StatusNone, status)
}

func TestApproveRequiresIssueIDWithoutList(t *testing.T) {
	_, err := runApproveCmd(t, "--dir", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issue id is required")
}

func TestApprovalStoreFor(t *testing.T) {
	assert.Nil(t, approvalStoreFor(nil))
	assert.Nil(t, approvalStoreFor(&config.WorkflowConfig{}))

	cfg := &config.WorkflowConfig{Approval: config.ApprovalConfig{RequirePlan: true}}
	assert.NotNil(t, approvalStoreFor(cfg))
}

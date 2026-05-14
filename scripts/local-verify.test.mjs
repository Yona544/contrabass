import assert from "node:assert/strict";
import { test } from "node:test";

import { buildPlan, GO_PACKAGES, resolveBunCommand } from "./local-verify.mjs";

test("local Go package set covers stable core packages", () => {
  for (const pkg of [
    "./cmd/contrabass",
    "./internal/config",
    "./internal/tracker",
    "./internal/tui",
    "./internal/web",
    "./internal/workspace",
  ]) {
    assert.ok(GO_PACKAGES.includes(pkg), `${pkg} should be in the local gate`);
  }
});

test("local Go package set skips known flaky Windows packages", () => {
  for (const pkg of [
    "./internal/agent",
    "./internal/orchestrator",
    "./internal/team",
    "./tests/e2e",
  ]) {
    assert.equal(GO_PACKAGES.includes(pkg), false, `${pkg} should stay out of the local gate`);
  }
});

test("default local verification plan runs Go, dashboard, and landing checks", () => {
  const plan = buildPlan([]);

  assert.deepEqual(
    plan.map((step) => step.name),
    ["go", "dashboard", "landing"],
  );
  assert.equal(plan[0].command, "go");
  assert.deepEqual(plan[0].args.slice(0, 2), ["test", "./cmd/contrabass"]);
  assert.deepEqual(plan[0].args.slice(-1), ["-count=1"]);
  assert.equal(plan[1].cwd, "packages/dashboard");
  assert.equal(plan[2].cwd, "packages/landing");
});

test("--go-only limits the verification plan to stable Go packages", () => {
  const plan = buildPlan(["--go-only"]);

  assert.deepEqual(
    plan.map((step) => step.name),
    ["go"],
  );
});

test("Bun resolver uses the Windows user install when bun is not on PATH", () => {
  const userProfile = "C:\\Users\\Operator";
  const expected = "C:\\Users\\Operator\\.bun\\bin\\bun.exe";

  assert.equal(
    resolveBunCommand({
      env: { USERPROFILE: userProfile },
      exists: (candidate) => candidate === expected,
      platform: "win32",
    }),
    expected,
  );
});

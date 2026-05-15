import assert from "node:assert/strict";
import { test } from "node:test";

import { buildPlan, GO_PACKAGES, main, resolveBunCommand } from "./local-verify.mjs";

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

test("--go-only preserves the stable Go command shape and ordering", () => {
  const plan = buildPlan(["--go-only"], { bunCommand: "bun-test" });

  assert.deepEqual(plan, [
    {
      name: "go",
      cwd: ".",
      command: "go",
      args: ["test", ...GO_PACKAGES, "-count=1"],
    },
  ]);
});

test("--plan prints deterministic commands without running them", () => {
  const output = captureConsole(() => {
    assert.equal(main(["--go-only", "--plan"]), 0);
  });

  assert.deepEqual(output.stdout, [
    `go: (.) go test ${GO_PACKAGES.join(" ")} -count=1`,
  ]);
  assert.deepEqual(output.stderr, []);
});

test("unknown flags return a usage error without running checks", () => {
  const output = captureConsole(() => {
    assert.equal(main(["--skip-dashboard", "--unknown"]), 2);
  });

  assert.deepEqual(output.stdout, []);
  assert.deepEqual(output.stderr, ["unknown option: --unknown"]);
});

test("skipping every check returns a usage error", () => {
  const output = captureConsole(() => {
    assert.equal(main(["--skip-go", "--skip-dashboard", "--skip-landing"]), 2);
  });

  assert.deepEqual(output.stdout, []);
  assert.deepEqual(output.stderr, ["nothing to verify; all checks were skipped"]);
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

test("Bun resolver prefers BUN_INSTALL before the user install", () => {
  const bunInstall = "C:\\tools\\bun";
  const userProfile = "C:\\Users\\Operator";
  const expected = "C:\\tools\\bun\\bin\\bun.exe";
  const fallback = "C:\\Users\\Operator\\.bun\\bin\\bun.exe";

  assert.equal(
    resolveBunCommand({
      env: { BUN_INSTALL: bunInstall, USERPROFILE: userProfile },
      exists: (candidate) => candidate === expected || candidate === fallback,
      platform: "win32",
    }),
    expected,
  );
});

function captureConsole(fn) {
  const originalLog = console.log;
  const originalError = console.error;
  const stdout = [];
  const stderr = [];

  console.log = (...args) => stdout.push(args.join(" "));
  console.error = (...args) => stderr.push(args.join(" "));
  try {
    fn();
  } finally {
    console.log = originalLog;
    console.error = originalError;
  }

  return { stdout, stderr };
}

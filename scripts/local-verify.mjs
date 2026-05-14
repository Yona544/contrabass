#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";

export const GO_PACKAGES = Object.freeze([
  "./cmd/contrabass",
  "./internal/config",
  "./internal/hooks",
  "./internal/hub",
  "./internal/ipc",
  "./internal/logging",
  "./internal/timeline",
  "./internal/tmux",
  "./internal/tracker",
  "./internal/tui",
  "./internal/types",
  "./internal/update",
  "./internal/web",
  "./internal/workspace",
]);

const ALLOWED_FLAGS = new Set([
  "--go-only",
  "--help",
  "--plan",
  "--skip-dashboard",
  "--skip-go",
  "--skip-landing",
]);

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

export function resolveBunCommand({
  env = process.env,
  exists = existsSync,
  platform = process.platform,
} = {}) {
  const candidates = [];
  if (platform === "win32") {
    if (env.BUN_INSTALL) {
      candidates.push(path.win32.join(env.BUN_INSTALL, "bin", "bun.exe"));
    }
    if (env.USERPROFILE) {
      candidates.push(path.win32.join(env.USERPROFILE, ".bun", "bin", "bun.exe"));
    }
  } else {
    if (env.BUN_INSTALL) {
      candidates.push(path.posix.join(env.BUN_INSTALL, "bin", "bun"));
    }
    if (env.HOME) {
      candidates.push(path.posix.join(env.HOME, ".bun", "bin", "bun"));
    }
  }

  return candidates.find((candidate) => exists(candidate)) ?? "bun";
}

export function buildPlan(argv = [], options = {}) {
  const flags = new Set(argv);
  const bunCommand = options.bunCommand ?? "bun";
  const goOnly = flags.has("--go-only");
  const skipGo = flags.has("--skip-go");
  const skipDashboard = goOnly || flags.has("--skip-dashboard");
  const skipLanding = goOnly || flags.has("--skip-landing");

  const plan = [];
  if (!skipGo) {
    plan.push({
      name: "go",
      cwd: ".",
      command: "go",
      args: ["test", ...GO_PACKAGES, "-count=1"],
    });
  }
  if (!skipDashboard) {
    plan.push({
      name: "dashboard",
      cwd: "packages/dashboard",
      command: bunCommand,
      args: ["test"],
    });
  }
  if (!skipLanding) {
    plan.push({
      name: "landing",
      cwd: "packages/landing",
      command: bunCommand,
      args: ["run", "check"],
    });
  }

  return plan;
}

function validateFlags(argv) {
  const unknown = argv.filter((arg) => arg.startsWith("--") && !ALLOWED_FLAGS.has(arg));
  if (unknown.length > 0) {
    throw new Error(`unknown option: ${unknown.join(", ")}`);
  }
}

function formatCommand(step) {
  return [step.command, ...step.args].join(" ");
}

function printHelp() {
  console.log(`Usage: node scripts/local-verify.mjs [options]

Runs the stable local verification gate. By default this checks the stable Go
package set, dashboard tests, and landing checks.

Options:
  --go-only          Run only the stable Go package set
  --skip-go         Skip Go package tests
  --skip-dashboard  Skip dashboard tests
  --skip-landing    Skip landing checks
  --plan            Print the commands without running them
  --help            Show this help
`);
}

function runStep(step) {
  console.log(`\n==> ${formatCommand(step)}`);
  const result = spawnSync(step.command, step.args, {
    cwd: path.resolve(repoRoot, step.cwd),
    stdio: "inherit",
    shell: process.platform === "win32" && !path.isAbsolute(step.command),
  });

  if (result.error) {
    console.error(`failed to start ${step.name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    console.error(`${step.name} stopped by signal ${result.signal}`);
    return 1;
  }
  return result.status ?? 1;
}

export function main(argv = process.argv.slice(2)) {
  try {
    validateFlags(argv);
  } catch (err) {
    console.error(err.message);
    return 2;
  }

  if (argv.includes("--help")) {
    printHelp();
    return 0;
  }

  const plan = buildPlan(argv, { bunCommand: resolveBunCommand() });
  if (plan.length === 0) {
    console.error("nothing to verify; all checks were skipped");
    return 2;
  }

  if (argv.includes("--plan")) {
    for (const step of plan) {
      console.log(`${step.name}: (${step.cwd}) ${formatCommand(step)}`);
    }
    return 0;
  }

  for (const step of plan) {
    const status = runStep(step);
    if (status !== 0) {
      return status;
    }
  }

  return 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exitCode = main();
}

#!/usr/bin/env node

// Cross-platform replacement for the former dev-dashboard.sh: starts the
// Contrabass backend (internal board) and the Vite dashboard dev server
// together, then shuts both down when either exits or on Ctrl-C.

import { spawn, spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { resolveBunCommand } from "./local-verify.mjs";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const FRONTEND_HOST = process.env.FRONTEND_HOST || "127.0.0.1";
const FRONTEND_PORT = process.env.FRONTEND_PORT || "5173";
const BACKEND_HOST = process.env.BACKEND_HOST || "127.0.0.1";
const BACKEND_PORT = process.env.BACKEND_PORT || "8080";

const RUNTIME_DIR =
  process.env.CONTRABASS_DASHBOARD_RUNTIME_DIR ||
  path.join(tmpdir(), "contrabass-dashboard-dev");
const BOARD_DIR = process.env.CONTRABASS_DASHBOARD_BOARD_DIR || path.join(RUNTIME_DIR, "board");
const WORKFLOW_FILE = path.join(RUNTIME_DIR, "workflow.dashboard.md");
const LOG_FILE = path.join(RUNTIME_DIR, "contrabass.log");

const BACKEND_URL = `http://${BACKEND_HOST}:${BACKEND_PORT}`;
const FRONTEND_URL = `http://${FRONTEND_HOST}:${FRONTEND_PORT}`;

function requireCommand(command, args) {
  const result = spawnSync(command, args, {
    stdio: "ignore",
    shell: process.platform === "win32" && !path.isAbsolute(command),
  });
  if (result.error || result.status !== 0) {
    console.error(`Missing required command: ${command}`);
    process.exit(1);
  }
}

function yamlQuote(value) {
  return `"${value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function writeWorkflow() {
  mkdirSync(RUNTIME_DIR, { recursive: true });

  writeFileSync(
    WORKFLOW_FILE,
    `---
max_concurrency: 1
poll_interval_ms: 5000
max_retry_backoff_ms: 30000
model: codex-mini
agent_timeout_ms: 120000
stall_timeout_ms: 60000
tracker:
  type: internal
  board_dir: ${yamlQuote(BOARD_DIR)}
agent:
  type: codex
codex:
  binary_path: codex app-server
  approval_policy: auto-edit
  sandbox: none
---
# Dashboard backend preview

Temporary internal board used for local dashboard API/SSE development.
`,
  );
}

async function waitForUrl(url, name, attempts = 60) {
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      await fetch(url, { signal: AbortSignal.timeout(2000) });
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  }
  throw new Error(`Timed out waiting for ${name} at ${url}`);
}

const children = [];

function stopChild(child) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  if (process.platform === "win32") {
    // child.kill only reaches the direct child; taskkill /T removes the
    // whole tree (go run and bun both spawn grandchildren).
    spawnSync("taskkill", ["/pid", String(child.pid), "/T", "/F"], { stdio: "ignore" });
  } else {
    child.kill("SIGTERM");
  }
}

let shuttingDown = false;
function shutdown(code) {
  if (shuttingDown) {
    return;
  }
  shuttingDown = true;
  for (const child of children) {
    stopChild(child);
  }
  process.exitCode = code;
}

async function main() {
  requireCommand("go", ["version"]);
  const bunCommand = resolveBunCommand();
  requireCommand(bunCommand, ["--version"]);

  writeWorkflow();

  process.on("SIGINT", () => shutdown(0));
  process.on("SIGTERM", () => shutdown(0));

  console.log(`Starting Contrabass backend on ${BACKEND_URL}`);
  const backend = spawn(
    "go",
    [
      "run",
      "./cmd/contrabass",
      "--config",
      WORKFLOW_FILE,
      "--no-tui",
      "--port",
      BACKEND_PORT,
      "--log-file",
      LOG_FILE,
    ],
    { cwd: repoRoot, stdio: "inherit" },
  );
  children.push(backend);
  backend.on("exit", (code) => shutdown(code ?? 1));

  await waitForUrl(`${BACKEND_URL}/api/v1/state`, "backend");

  console.log(`Starting Dashboard frontend on ${FRONTEND_URL}`);
  const frontend = spawn(
    bunCommand,
    ["run", "dev", "--", "--host", FRONTEND_HOST, "--port", FRONTEND_PORT],
    {
      cwd: path.join(repoRoot, "packages", "dashboard"),
      stdio: "inherit",
      env: { ...process.env, CONTRABASS_BACKEND_URL: BACKEND_URL },
      shell: process.platform === "win32" && !path.isAbsolute(bunCommand),
    },
  );
  children.push(frontend);
  frontend.on("exit", (code) => shutdown(code ?? 1));

  await waitForUrl(FRONTEND_URL, "frontend");

  console.log(`
Contrabass dashboard stack is ready.
  Frontend: ${FRONTEND_URL}
  Backend:  ${BACKEND_URL}
  Runtime:  ${RUNTIME_DIR}

Press Ctrl-C to stop both processes.
`);
}

main().catch((err) => {
  console.error(err.message);
  shutdown(1);
});

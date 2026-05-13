import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const expectedVersion = "1.6.5-rc.26";
const gitnexusCommand = "node scripts/gitnexus.mjs";
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const packageJSON = JSON.parse(
  readFileSync(path.join(root, "package.json"), "utf8"),
);
const agentsMarkdown = readFileSync(path.join(root, "AGENTS.md"), "utf8");
const cliSkillMarkdown = readFileSync(
  path.join(root, ".claude", "skills", "gitnexus", "gitnexus-cli", "SKILL.md"),
  "utf8",
);
const wrapperSource = readFileSync(path.join(root, "scripts", "gitnexus.mjs"), "utf8");

function assert(condition, message) {
  if (!condition) {
    console.error(message);
    process.exit(1);
  }
}

const expectedScripts = {
  gitnexus: gitnexusCommand,
  "gitnexus:analyze": `${gitnexusCommand} analyze`,
  "gitnexus:analyze:embeddings": `${gitnexusCommand} analyze --embeddings`,
  "gitnexus:detect": `${gitnexusCommand} detect-changes`,
  "gitnexus:doctor": `${gitnexusCommand} doctor`,
  "gitnexus:status": `${gitnexusCommand} status`,
  "test:gitnexus": "node scripts/gitnexus-smoke.mjs",
};

for (const [name, command] of Object.entries(expectedScripts)) {
  assert(
    packageJSON.scripts?.[name] === command,
    `package.json script ${name} must be ${JSON.stringify(command)}`,
  );
}

const npmBin = "npm";
const npmExecArgs = [
  "exec",
  "--yes",
  "--package",
  `gitnexus@${expectedVersion}`,
  "--",
  "gitnexus",
];
const help = spawnSync(npmBin, [...npmExecArgs, "--help"], {
  cwd: root,
  encoding: "utf8",
  shell: process.platform === "win32",
});
assert(help.status === 0, help.stderr || help.stdout);
assert(
  help.stdout.includes("GitNexus local CLI and MCP server"),
  "GitNexus help output did not match the expected CLI",
);

const doctor = spawnSync(npmBin, [...npmExecArgs, "doctor"], {
  cwd: root,
  encoding: "utf8",
  shell: process.platform === "win32",
});
assert(doctor.status === 0, doctor.stderr || doctor.stdout);
assert(
  wrapperSource.includes(`gitnexusVersion = "${expectedVersion}"`),
  `scripts/gitnexus.mjs must pin gitnexus to ${expectedVersion}`,
);

assert(
  agentsMarkdown.includes("npm run gitnexus:analyze"),
  "AGENTS.md must tell agents to use the pinned GitNexus analyze script",
);
assert(
  agentsMarkdown.includes("npm run gitnexus:detect"),
  "AGENTS.md must tell agents to use the pinned GitNexus detect script",
);
assert(
  cliSkillMarkdown.includes("npm run gitnexus:analyze"),
  "GitNexus CLI skill must document the pinned analyze script",
);
assert(
  cliSkillMarkdown.includes("npm run gitnexus:status"),
  "GitNexus CLI skill must document the pinned status script",
);

console.log(`GitNexus ${expectedVersion} smoke test passed`);

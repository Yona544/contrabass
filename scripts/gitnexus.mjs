import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const gitnexusVersion = "1.6.5-rc.26";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

let passthroughArgs = process.argv.slice(2);
if (passthroughArgs[0] === "--") {
  passthroughArgs = passthroughArgs.slice(1);
}
if (
  ["augment", "cypher", "query"].includes(passthroughArgs[0]) &&
  passthroughArgs.length > 2
) {
  passthroughArgs = [passthroughArgs[0], passthroughArgs.slice(1).join(" ")];
}

const npmArgs = [
  "exec",
  "--yes",
  "--package",
  `gitnexus@${gitnexusVersion}`,
  "--",
  "gitnexus",
  ...passthroughArgs,
];

function quoteWindowsArg(arg) {
  if (!/[\s"]/u.test(arg)) {
    return arg;
  }
  return `"${arg.replace(/"/gu, '\\"')}"`;
}

const result =
  process.platform === "win32"
    ? spawnSync(["npm", ...npmArgs].map(quoteWindowsArg).join(" "), {
        cwd: process.cwd(),
        stdio: "inherit",
        shell: true,
      })
    : spawnSync("npm", npmArgs, {
        cwd: process.cwd(),
        stdio: "inherit",
        shell: false,
      });

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

const status = result.status ?? 1;
if (status === 0 && passthroughArgs[0] === "analyze") {
  patchGeneratedDocs();
}

process.exit(status);

function patchGeneratedDocs() {
  patchFile(path.join(root, "AGENTS.md"), (content) =>
    content.replace(
      /^> If any GitNexus tool warns the index is stale, run `.*?` in terminal first\.$/m,
      "> If any GitNexus tool warns the index is stale, run `npm run gitnexus:analyze` in terminal first. If semantic `query` results are needed on Windows, run `npm run gitnexus:analyze:embeddings`; Windows may still warn that FTS indexes are missing while using exact-scan semantic fallback. Before committing local edits, run `npm run gitnexus:detect`.",
    ),
  );

  patchFile(
    path.join(root, ".claude", "skills", "gitnexus", "gitnexus-cli", "SKILL.md"),
    (content) => {
      let next = content
        .replace(
          "All commands work via `npx` - no global install required.",
          "Use the repository-pinned npm scripts from the project root. The root `package.json` pins the GitNexus CLI through `scripts/gitnexus.mjs`, so avoid unpinned `npx gitnexus`.",
        )
        .replace(
          "All commands work via `npx` — no global install required.",
          "Use the repository-pinned npm scripts from the project root. The root `package.json` pins the GitNexus CLI through `scripts/gitnexus.mjs`, so avoid unpinned `npx gitnexus`.",
        )
        .replace(/npx gitnexus analyze/g, "npm run gitnexus:analyze")
        .replace(/npx gitnexus status/g, "npm run gitnexus:status")
        .replace(/npx gitnexus clean/g, "npm run gitnexus -- clean")
        .replace(/npx gitnexus wiki/g, "npm run gitnexus -- wiki")
        .replace(/npx gitnexus list/g, "npm run gitnexus -- list");

      if (!next.includes("npm run gitnexus:analyze:embeddings")) {
        next = next.replace(
          "\n**When to run:**",
          "\nFor this Windows repo, run `npm run gitnexus:analyze:embeddings` when semantic `query` quality matters. GitNexus may still warn that FTS indexes are missing on Windows; with embeddings present it falls back to exact-scan semantic search.\n\n**When to run:**",
        );
      }
      return next;
    },
  );
}

function patchFile(file, transform) {
  const current = readFileSync(file, "utf8");
  const next = transform(current);
  if (next !== current) {
    writeFileSync(file, next, "utf8");
  }
}

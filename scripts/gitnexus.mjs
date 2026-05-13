import { spawnSync } from "node:child_process";

export const gitnexusVersion = "1.6.5-rc.26";

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

process.exit(result.status ?? 1);

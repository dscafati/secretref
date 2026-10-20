#!/usr/bin/env node
"use strict";

const { spawn } = require("node:child_process");

const platform = `${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "secretref.exe" : "secretref";
const target = `@secretref/${platform}/bin/${exe}`;

let binary;
try {
  binary = require.resolve(target);
} catch {
  console.error(
    [
      `secretref: no prebuilt binary found for ${platform} (expected package @secretref/${platform}).`,
      "",
      "This usually means optional dependencies were skipped (for example --omit=optional)",
      "or this platform is not supported. Try reinstalling:",
      "  npm install --include=optional secretref",
      "or download a release binary from https://github.com/dscafati/secretref/releases",
    ].join("\n"),
  );
  process.exit(1);
}

const child = spawn(binary, process.argv.slice(2), { stdio: "inherit" });

const os = require("node:os");

// Ctrl+C: on Windows the console delivers it to every attached process, and
// child.kill() would force-terminate the child, so SIGINT is only swallowed
// here. On POSIX with a TTY the foreground process group already delivers it.
// Without a TTY (e.g. `kill -INT` on this process) we forward it ourselves.
const forwarded = ["SIGINT", "SIGTERM", "SIGHUP"];
const handlers = forwarded.map((sig) => {
  let handler;
  if (sig === "SIGINT" && process.platform === "win32") {
    handler = () => {};
  } else if (sig === "SIGINT" && process.stdin.isTTY) {
    handler = () => {};
  } else if (process.platform === "win32") {
    handler = () => {};
  } else {
    handler = () => {
      try {
        child.kill(sig);
      } catch {
        // child already gone
      }
    };
  }
  process.on(sig, handler);
  return [sig, handler];
});

child.on("error", (err) => {
  console.error(`secretref: failed to start ${binary}: ${err.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  for (const [sig, handler] of handlers) {
    process.removeListener(sig, handler);
  }
  if (signal) {
    const num = os.constants.signals[signal];
    process.exit(num ? 128 + num : 1);
  }
  process.exit(code ?? 1);
});

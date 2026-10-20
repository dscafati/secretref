#!/usr/bin/env node
"use strict";

const { version } = require("../package.json");

const args = process.argv.slice(2);

if (args[0] === "--version" || args[0] === "-v") {
  console.log(version);
  process.exit(0);
}

console.log(`secretref ${version}

Resolve secrets from vaults using reference comments in your .env files.

  # secretref: vault://kv/app/db#url
  DATABASE_URL=postgres://localhost:5432/dev

  secretref run -- node server.js

This is an early preview. The resolver is in active development.`);
process.exit(args.length === 0 || args[0] === "--help" || args[0] === "-h" ? 0 : 1);

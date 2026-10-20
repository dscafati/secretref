# 0008. GCP over REST, not gRPC

Status: Accepted

Date: 2026-10-20

## Context

The official GCP Secret Manager client uses gRPC and pulls in a large dependency tree. That grows the binary, slows builds, and widens the supply-chain surface. We need one call: access a secret version.

## Decision

The `gcpsm` provider calls the Secret Manager REST API with `net/http`. It gets Application Default Credentials from `golang.org/x/oauth2`. It does not import the Google Cloud client libraries.

## Consequences

- The binary and dependency list stay small.
- We maintain a small REST client and its response parsing.
- Tests run against a fake HTTP server, so no GCP account is needed.
- If we need features beyond reading versions, we revisit this decision.

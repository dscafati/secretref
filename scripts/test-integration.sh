#!/usr/bin/env bash
# Runs the provider integration tests (build tag `integration`) against local
# stand-ins only. It never uses a real cloud account.
#
#   aws-sm   Moto's Secrets Manager emulator in a container started with
#            $CONTAINER (podman or docker), on a random loopback port.
#   keyring  On Linux with dbus-run-session and gnome-keyring-daemon installed,
#            a throwaway Secret Service on a private session bus. Otherwise the
#            real OS keyring (macOS Keychain, Windows Credential Manager), or a
#            skip when none answers.
#
# The container and the throwaway keyring are always removed on exit.
set -euo pipefail

container="${CONTAINER:-podman}"
go="${GO:-go}"
# Tag and digest move together. The digest is the multi-arch index.
moto_image="docker.io/motoserver/moto:5.2.4@sha256:120b1589c6df7e35d2092935f6ff724486bb2c4189d0e36ce795baba1400ffd2"

self="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
cd "$(dirname "$self")/.."

# Re-run inside a private session bus so the keyring test gets its own Secret
# Service and cannot read or write the developer's keyring.
if [ "$(uname -s)" = Linux ] && [ -z "${SECRETREF_PRIVATE_BUS:-}" ] &&
  command -v dbus-run-session >/dev/null 2>&1 &&
  command -v gnome-keyring-daemon >/dev/null 2>&1; then
  SECRETREF_PRIVATE_BUS=1 exec dbus-run-session -- bash "$self" "$@"
fi

scratch="$(mktemp -d)"
container_id=""
cleanup() {
  if [ -n "$container_id" ]; then
    "$container" rm -f "$container_id" >/dev/null 2>&1 || true
  fi
  rm -rf "$scratch"
}
trap cleanup EXIT

if [ -n "${SECRETREF_PRIVATE_BUS:-}" ]; then
  # The keyring files and control socket live in $scratch. Only the daemon sees
  # these XDG paths; podman needs the real XDG_RUNTIME_DIR. The password is a
  # fixed test value for a keyring that is deleted on exit.
  mkdir -m 700 "$scratch/data" "$scratch/run"
  printf 'secretref-integration' |
    XDG_DATA_HOME="$scratch/data" XDG_RUNTIME_DIR="$scratch/run" \
      gnome-keyring-daemon --unlock --components=secrets >/dev/null
  echo "test-integration: keyring: throwaway gnome-keyring on a private session bus"
else
  echo "test-integration: keyring: using the OS keyring of this session"
fi

container_id="$("$container" run --detach --rm --publish 127.0.0.1::5000 "$moto_image")"
addr="$("$container" port "$container_id" 5000/tcp | head -n 1)"
endpoint="http://$addr"

ready=""
for _ in $(seq 1 60); do
  if curl --silent --fail --output /dev/null "$endpoint/moto-api/"; then
    ready=1
    break
  fi
  sleep 1
done
if [ -z "$ready" ]; then
  echo "test-integration: Moto did not answer at $endpoint within 60s" >&2
  "$container" logs "$container_id" >&2 || true
  exit 1
fi
echo "test-integration: aws-sm: Moto at $endpoint"

# Dummy credentials for the emulator. Shared config, profiles, and instance
# metadata are disabled so nothing can pick up a real AWS identity.
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN AWS_ROLE_ARN AWS_WEB_IDENTITY_TOKEN_FILE
export AWS_ENDPOINT_URL="$endpoint"
export AWS_ACCESS_KEY_ID=testing
export AWS_SECRET_ACCESS_KEY=testing
export AWS_REGION=us-east-1
export AWS_CONFIG_FILE=/dev/null
export AWS_SHARED_CREDENTIALS_FILE=/dev/null
export AWS_EC2_METADATA_DISABLED=true
export SECRETREF_REQUIRE_AWS_EMULATOR=1
if [ -n "${SECRETREF_PRIVATE_BUS:-}" ]; then
  export SECRETREF_REQUIRE_KEYRING=1
fi

"$go" test -tags integration -count=1 ./internal/provider/...

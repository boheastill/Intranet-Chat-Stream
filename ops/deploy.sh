#!/usr/bin/env bash
# Build for the server with the build version stamped in, so the journal records
# which build is actually running (otherwise every deploy looks identical).
#
#   ./ops/deploy.sh <git-ref-or-description>
set -euo pipefail
cd "$(dirname "$0")/.."

VER="${1:-$(git rev-parse --short HEAD)}"
DIRTY=""
git diff --quiet || DIRTY="-dirty"

GOOS=linux GOARCH=amd64 go build \
  -ldflags "-X ics/bus.buildVersion=${VER}${DIRTY}" \
  -o /tmp/ics-deploy .

echo "built: ${VER}${DIRTY}"

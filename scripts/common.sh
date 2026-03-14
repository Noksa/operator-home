#!/usr/bin/env bash
set -euo pipefail

# Project root (two levels up from scripts/)
PROJECT_DIR="$(cd "$(dirname "$(dirname "$(realpath "$0")")")" && pwd)"

# Cyberpunk theme — auto-download if missing
CYBER_CACHE="${PROJECT_DIR}/.cyber.sh"
CYBER_URL="https://raw.githubusercontent.com/Noksa/install-scripts/main/cyberpunk.sh"

if [ ! -f "${CYBER_CACHE}" ]; then
  curl -s "${CYBER_URL}" > "${CYBER_CACHE}"
fi

# shellcheck disable=SC1090
source "${CYBER_CACHE}"

trap cyber_trap SIGINT SIGTERM

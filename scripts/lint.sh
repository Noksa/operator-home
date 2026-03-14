#!/usr/bin/env bash
# shellcheck disable=SC1091
source "$(dirname "$(dirname "$(realpath "$0")")")/scripts/common.sh"

cyber_step "Lint"

# ── go mod tidy ───────────────────────────────────────────────────
cyber_log "Running go mod tidy..."
go mod tidy
cyber_ok "Modules tidy"

# ── go fmt ────────────────────────────────────────────────────────
cyber_log "Running go fmt..."
go fmt ./...
cyber_ok "Code formatted"

# ── go vet ────────────────────────────────────────────────────────
cyber_log "Running go vet..."
go vet ./...
cyber_ok "Vet passed"

# ── modernize ────────────────────────────────────────────────────
cyber_log "Running modernize..."
go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -fix ./...
cyber_ok "Modernize passed"

# ── golangci-lint ─────────────────────────────────────────────────
if command -v golangci-lint &>/dev/null; then
  cyber_log "Running golangci-lint..."
  golangci-lint run ./...
  cyber_ok "golangci-lint passed"
else
  cyber_warn "golangci-lint not found — skipping"
  echo -e "${CYBER_D}┌─────────────────────────────────────┐${CYBER_X}"
  echo -e "${CYBER_D}│${CYBER_X} ${CYBER_W}Install${CYBER_X} ${CYBER_C}→${CYBER_X} ${CYBER_G}go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest${CYBER_X}"
  echo -e "${CYBER_D}└─────────────────────────────────────┘${CYBER_X}"
fi

cyber_ok "All lint checks passed"

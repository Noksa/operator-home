SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
ROOT_DIR = $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.DEFAULT_GOAL = help

# ── Go configuration ──────────────────────────────────────────────
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# ── Test configuration ────────────────────────────────────────────
GINKGO       := $(GOBIN)/ginkgo
GINKGO_PROCS ?= 3
GINKGO_FLAGS ?= --silence-skips --procs=$(GINKGO_PROCS)

# ── Cyberpunk theme ──────────────────────────────────────────────
CYBER_CACHE := .cyber.sh
CYBER_URL   := https://raw.githubusercontent.com/Noksa/install-scripts/main/cyberpunk.sh

$(CYBER_CACHE):
	@curl -s $(CYBER_URL) > $(CYBER_CACHE)

# ── Macros ────────────────────────────────────────────────────────
define ensure_ginkgo
	@if [ ! -f $(GINKGO) ]; then \
		source $(CYBER_CACHE) && cyber_log "Installing ginkgo CLI..."; \
		go install github.com/onsi/ginkgo/v2/ginkgo@latest; \
		source $(CYBER_CACHE) && cyber_ok "ginkgo installed"; \
	fi
endef

define run_tests
	$(ensure_ginkgo)
	@source $(CYBER_CACHE) && cyber_step "Running tests: $(1)"
	@$(GINKGO) $(GINKGO_FLAGS) $(if $(2),--focus "$(2)",) $(1)
	@source $(CYBER_CACHE) && cyber_ok "Tests passed"
endef

##@ General

.PHONY: help
help: $(CYBER_CACHE) ## Show help
	@source $(CYBER_CACHE) && { \
		echo ""; \
		echo -e "$${CYBER_D}╔═══════════════════════════════════════╗$${CYBER_X}"; \
		echo -e "$${CYBER_D}║$${CYBER_X}  $${CYBER_M}🦋$${CYBER_X} $${CYBER_B}$${CYBER_C}OPERATOR-HOME$${CYBER_X}"; \
		echo -e "$${CYBER_D}╚═══════════════════════════════════════╝$${CYBER_X}"; \
	}
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[36mUsage:\033[0m make \033[35m<target>\033[0m\n\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m \033[37m%s\033[0m\n", $$1, $$2 } /^##@/ { printf "\n\033[35m⚡ %s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo ""

.PHONY: cyber-update
cyber-update: ## Update cyberpunk theme
	@rm -f $(CYBER_CACHE)
	@curl -s $(CYBER_URL) > $(CYBER_CACHE)
	@source $(CYBER_CACHE) && cyber_ok "Cyberpunk theme updated"

##@ Development

.PHONY: lint
lint: ## Run all linters (tidy, fmt, vet, modernize, golangci-lint)
	@./scripts/lint.sh

##@ Testing

.PHONY: test
test: $(CYBER_CACHE) ## Run all unit tests
	$(call run_tests,./...)

.PHONY: test-pkg
test-pkg: $(CYBER_CACHE) ## Run tests for a specific package (PKG=./pkg/...)
	$(call run_tests,$(PKG))

.PHONY: test-focus
test-focus: $(CYBER_CACHE) ## Run focused tests (PKG=./pkg/... FOCUS="pattern")
	$(call run_tests,$(PKG),$(FOCUS))

.PHONY: test-race
test-race: $(CYBER_CACHE) ## Run tests with race detector
	$(ensure_ginkgo)
	@source $(CYBER_CACHE) && cyber_step "Running tests with race detector"
	@$(GINKGO) $(GINKGO_FLAGS) --race ./...
	@source $(CYBER_CACHE) && cyber_ok "Race-free"

.PHONY: test-verbose
test-verbose: $(CYBER_CACHE) ## Run tests with verbose output
	$(ensure_ginkgo)
	@source $(CYBER_CACHE) && cyber_step "Running tests (verbose)"
	@$(GINKGO) -v --silence-skips ./...
	@source $(CYBER_CACHE) && cyber_ok "Tests passed"

.PHONY: test-coverage
test-coverage: $(CYBER_CACHE) ## Run tests with coverage report
	@source $(CYBER_CACHE) && cyber_step "Running tests with coverage"
	@go test -count=1 -race -coverprofile=cover.out ./...
	@go tool cover -html=cover.out -o cover.html
	@source $(CYBER_CACHE) && cyber_ok "Coverage report: cover.html"

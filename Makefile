# One entry point for the checks CI runs: `make check` runs them all and
# `make fmt` formats Go and the web app in place; `make help` lists the targets. The tools
# are pinned in mise.toml; with mise installed they run through it, and
# otherwise from PATH.
MISE := $(if $(shell command -v mise 2>/dev/null),mise exec --)

WEB_DEPS := web/node_modules/.package-lock.json
CODEGEN_DEPS := web/codegen/node_modules/.package-lock.json

.PHONY: help check fmt lint lint-go lint-web lint-other test test-go test-web api

help: ## list the targets
	@awk -F':.*## ' '/^[a-z-]+:.*## / {printf "%-11s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: lint test api ## everything CI checks

fmt: $(WEB_DEPS) ## format Go and the web app in place (a new migration: sqlfluff fix <file>)
	$(MISE) golangci-lint fmt
	cd web && $(MISE) npm run --silent format

lint: lint-go lint-web lint-other ## every linter and format check

lint-go: ## golangci-lint, formatting included
	$(MISE) golangci-lint run

lint-web: $(WEB_DEPS) ## oxlint, Prettier and tsc for the web app
	cd web && $(MISE) npm run --silent lint
	cd web && $(MISE) npm run --silent format:check
	cd web && $(MISE) npm run --silent typecheck

lint-other: ## EditorConfig, SQL, Dockerfile, workflows, YAML and Markdown
	$(MISE) editorconfig-checker
	$(MISE) sqlfluff lint .
	$(MISE) hadolint Dockerfile
	$(MISE) yamllint -s .
	$(MISE) markdownlint-cli2
	@if [ -d .github/workflows ]; then \
		echo '$(MISE) actionlint && $(MISE) zizmor .'; \
		$(MISE) actionlint && $(MISE) zizmor .; \
	fi

test: test-go test-web ## Go tests with the race detector, then Vitest

test-go:
	$(MISE) go test -race ./...

test-web: $(WEB_DEPS)
	cd web && $(MISE) npm test --silent

api: $(CODEGEN_DEPS) ## the generated TypeScript types match api/openapi.json
	cd web/codegen && $(MISE) npm run --silent check:api

$(WEB_DEPS): web/package-lock.json
	cd web && $(MISE) npm ci --no-audit --no-fund

$(CODEGEN_DEPS): web/codegen/package-lock.json
	cd web/codegen && $(MISE) npm ci --no-audit --no-fund

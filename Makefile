# One entry point for the checks: `make check` runs what CI checks, except
# a few that only CI runs (docs/conventions.md), and `make fmt` formats Go
# and the web app in place; `make help` lists the targets. The tools are
# pinned in mise.toml; with mise installed they run through it, and
# otherwise from PATH.
MISE := $(if $(shell command -v mise 2>/dev/null),mise exec --)

WEB_DEPS := web/node_modules/.package-lock.json
CODEGEN_DEPS := web/codegen/node_modules/.package-lock.json

.PHONY: help check fmt lint lint-go lint-web lint-webui lint-other test test-go test-web test-webui webui-build api audit audit-go audit-npm

help: ## list the targets
	@awk -F':.*## ' '/^[a-z-]+:.*## / {printf "%-11s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: lint test test-webui api audit ## what CI checks, except a few that only CI runs (docs/conventions.md)

fmt: $(WEB_DEPS) ## format Go and the web app in place (a new migration: sqlfluff fix <file>)
	$(MISE) golangci-lint fmt
	cd web && $(MISE) npm run --silent format

lint: lint-go lint-web lint-webui lint-other ## every linter and format check

lint-go: ## golangci-lint, formatting included
	$(MISE) golangci-lint run

lint-web: $(WEB_DEPS) ## oxlint, Prettier and tsc for the web app
	cd web && $(MISE) npm run --silent lint
	cd web && $(MISE) npm run --silent format:check
	cd web && $(MISE) npm run --silent typecheck

lint-webui: webui-build ## golangci-lint on the files behind the webui tag, against a fresh build
	$(MISE) golangci-lint run --build-tags webui

lint-other: ## EditorConfig, SQL, Dockerfile, shell scripts, workflows, YAML and Markdown
	$(MISE) editorconfig-checker
	$(MISE) sqlfluff lint .
	$(MISE) hadolint Dockerfile
	$(MISE) shellcheck scripts/*.sh
	$(MISE) yamllint -s .
	$(MISE) markdownlint-cli2
	@if [ -d .github/workflows ]; then \
		echo '$(MISE) actionlint && $(MISE) zizmor .'; \
		$(MISE) actionlint && $(MISE) zizmor .; \
	fi

test: test-go test-web ## Go tests, then Vitest, both with coverage thresholds

test-go: ## Go tests with the race detector; coverage must meet .testcoverage.yml
	$(MISE) go test -race -coverprofile=coverage.out ./...
	$(MISE) go-test-coverage --config .testcoverage.yml

test-web: $(WEB_DEPS) ## Vitest; coverage must meet the thresholds in web/vite.config.ts
	cd web && $(MISE) npm run --silent coverage

api: $(CODEGEN_DEPS) ## the generated TypeScript types match api/openapi.json
	cd web/codegen && $(MISE) npm run --silent check:api

test-webui: webui-build ## the Go tests behind the webui tag, against a fresh build
	$(MISE) go test -tags webui ./internal/webui/...

# One build of the web app per make run, shared by the webui targets, so
# make -j never empties dist under a running one.
webui-build: $(WEB_DEPS)
	cd web && $(MISE) npm run --silent build

audit: audit-go audit-npm ## known vulnerabilities in Go and npm dependencies

audit-go:
	$(MISE) govulncheck ./...

audit-npm: $(WEB_DEPS) $(CODEGEN_DEPS)
	cd web && $(MISE) npm audit --audit-level=high
	cd web/codegen && $(MISE) npm audit --audit-level=high

$(WEB_DEPS): web/package-lock.json
	cd web && $(MISE) npm ci --no-audit --no-fund

$(CODEGEN_DEPS): web/codegen/package-lock.json
	cd web/codegen && $(MISE) npm ci --no-audit --no-fund

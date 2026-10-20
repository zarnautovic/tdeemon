# One entry point for the checks: `make check` runs what CI checks, except
# a few that only CI runs (docs/conventions.md), and `make fmt` formats Go
# and the web app in place; `make help` lists the targets. The tools are
# pinned in mise.toml; with mise installed they run through it, and
# otherwise from PATH.
MISE := $(if $(shell command -v mise 2>/dev/null),mise exec --)

# Licences a dependency may have: permissive ones and MPL-2.0. Copyleft
# licences such as GPL would keep the project from being offered under
# another licence besides the AGPL.
LICENSES := MIT,MIT-0,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,0BSD,MPL-2.0,BlueOak-1.0.0,CC0-1.0,Unlicense,Python-2.0,CC-BY-4.0

WEB_DEPS := web/node_modules/.package-lock.json
CODEGEN_DEPS := web/codegen/node_modules/.package-lock.json

.PHONY: help check fmt lint lint-go lint-web lint-webui lint-other test test-go test-web test-webui webui-build api audit audit-go audit-npm licenses secrets next-version test-scripts

help: ## list the targets
	@awk -F':.*## ' '/^[a-z-]+:.*## / {printf "%-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

next-version: ## the version the next release would get (scripts/next-version.sh)
	@v=$$($(MISE) scripts/next-version.sh) && echo "$${v:-no release: nothing since the last one makes a release}"

check: lint test test-webui api audit licenses secrets ## what CI checks, except a few that only CI runs (docs/conventions.md)

fmt: $(WEB_DEPS) ## format Go and the web app in place (a new migration: sqlfluff fix <file>)
	$(MISE) golangci-lint fmt
	cd web && $(MISE) npm run --silent format

lint: lint-go lint-web lint-webui lint-other ## every linter and format check

lint-go: ## golangci-lint, formatting included
	$(MISE) golangci-lint run

lint-web: $(WEB_DEPS) ## oxlint, Prettier, tsc and the reasons of disable comments for the web app
	cd web && $(MISE) npm run --silent lint
	cd web && $(MISE) npm run --silent format:check
	cd web && $(MISE) npm run --silent typecheck
	scripts/check-disable-comments.sh

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

test: test-go test-web test-scripts ## Go tests and Vitest, both with coverage thresholds, then the script tests

test-go: ## Go tests with the race detector; coverage must meet .testcoverage.yml
	$(MISE) go test -race -coverprofile=coverage.out ./...
	$(MISE) go-test-coverage --config .testcoverage.yml

test-web: $(WEB_DEPS) ## Vitest; coverage must meet the thresholds in web/vite.config.ts
	cd web && $(MISE) npm run --silent coverage

test-scripts: ## the tests of the shell scripts (scripts/*-test.sh)
	$(MISE) scripts/next-version-test.sh
	scripts/check-disable-comments-test.sh

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

licenses: ## every Go and npm dependency has an allowed licence (LICENSES)
	$(MISE) osv-scanner scan source --config osv-scanner-licenses.toml --licenses="$(LICENSES)" \
		-L go.mod -L web/package-lock.json -L web/codegen/package-lock.json

secrets: ## no secrets anywhere in the git history
	$(MISE) gitleaks git --verbose --redact --no-banner --log-opts=--all .

$(WEB_DEPS): web/package-lock.json
	cd web && $(MISE) npm ci --no-audit --no-fund

$(CODEGEN_DEPS): web/codegen/package-lock.json
	cd web/codegen && $(MISE) npm ci --no-audit --no-fund

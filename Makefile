# prdash — development, install and test tasks.
# Requires Go 1.26+ and, for real data, authenticated `gh`/`glab`.

BINARY  := prdash
PKG     := ./cmd/prdash
BINDIR  ?= $(HOME)/.local/bin
CONFDIR ?= $(HOME)/.config/prdash
# Same version dbx/gitdash/tsk/vroom use; it runs via `go run`, with no global
# binary. With no .golangci.yml, golangci-lint applies its default set.
GOLANGCI_LINT_VERSION := v2.13.2
MUTATE_BASE ?= main

.DEFAULT_GOAL := help

.PHONY: help build install uninstall run print test fmt fmt-check vet lint check tidy clean config config-path mutate mutate-diff

help: ## Shows the available tasks
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Builds the binary in ./bin/prdash
	@mkdir -p bin
	go build -o bin/$(BINARY) $(PKG)

install: build ## Installs the binary in $(BINDIR)/prdash
	@mkdir -p "$(BINDIR)"
	install -m 0755 bin/$(BINARY) "$(BINDIR)/$(BINARY)"
	@echo "installed: $(BINDIR)/$(BINARY)"

uninstall: ## Removes the installed binary
	rm -f "$(BINDIR)/$(BINARY)"

run: ## Opens the TUI
	go run $(PKG)

print: ## Runs --print mode (no TUI, to check the pipeline)
	go run $(PKG) --print

test: ## Full runner: build + vet + gofmt + test with -race
	go build ./...
	go vet ./...
	@fmt_out=$$(gofmt -l $$(git ls-files '*.go')); if [ -n "$$fmt_out" ]; then \
		echo "gofmt pending in:"; echo "$$fmt_out"; exit 1; \
	fi
	go test -race -count=1 ./...

fmt: ## Formats the code
	gofmt -w .

fmt-check: ## Checks gofmt formatting without writing (fails if anything is pending)
	@out="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$out" ]; then echo "gofmt pending in:"; echo "$$out"; exit 1; fi

vet: ## Analyses the code
	go vet ./...

lint: vet fmt-check ## go vet + gofmt + golangci-lint (pinned version, always via go run)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Local gate equivalent to CI: build + lint + test. `install` (which copies to
# ~/.local/bin) is left out on purpose; `make install` is a separate step.
check: build lint test
	@echo "check OK"

tidy: ## Syncs go.mod/go.sum
	go mod tidy

audit-lang: ## Advisory: flags Spanish content in tracked files (ADR 0010)
	@bash scripts/audit-lang.sh

mutate: ## Mutation testing (gremlins) on the whole module — advisory, never blocks CI
	go tool gremlins unleash --workers 4 --timeout-coefficient 3 --output report.json

# gremlins silently falls back to the whole module when the diff is empty (base == HEAD),
# so fail fast instead of running a full-module run that looks diff-scoped.
mutate-diff: ## Mutation testing (gremlins) restricted to the diff vs main — advisory
	@if git diff --name-only $(MUTATE_BASE)...HEAD | grep -q '\.go$$'; then \
		go tool gremlins unleash --diff $(MUTATE_BASE) --workers 4 --timeout-coefficient 3 --output report.json; \
	else \
		echo "no .go changes vs $(MUTATE_BASE) - nothing to mutate"; \
	fi

clean: ## Removes the build artifacts
	rm -rf bin

config-path: ## Prints the expected XDG config path
	@echo "$(CONFDIR)/config.toml"

config: ## Creates config.toml if missing (requires GITLAB_HOST=self-managed-host)
	@mkdir -p "$(CONFDIR)"
	@if [ -f "$(CONFDIR)/config.toml" ]; then \
		echo "already there: $(CONFDIR)/config.toml"; exit 0; \
	fi; \
	if [ -z "$(GITLAB_HOST)" ]; then \
		echo "GITLAB_HOST missing — example: make config GITLAB_HOST=gitlab.mycompany.com"; exit 1; \
	fi; \
	{ \
		echo '# prdash — XDG config (defaults if the file is missing).'; \
		echo 'roots = ["~/dev"]'; \
		echo 'refresh_interval = "60s"'; \
		echo; \
		echo '[forge.github]'; \
		echo 'enabled = true'; \
		echo 'host = "github.com"'; \
		echo; \
		echo '[forge.gitlab]'; \
		echo 'enabled = true'; \
		echo "host = \"$(GITLAB_HOST)\""; \
		echo '# REST base of the instance (glab resolves host and base on its own);'; \
		echo '# root by default. If the instance lives in a subfolder, use'; \
		echo '# api_base = "/git/api/v4/" or clone_base = "git" (relative URL'; \
		echo '# at the root of the clone/web; clone_base = "/" forces the root).'; \
		echo 'api_base = "/api/v4/"'; \
		echo; \
		echo '[forge.bitbucket]'; \
		echo 'enabled = false'; \
		echo; \
		echo '# Commands of the review panes. If you define a key, its value is'; \
		echo '# the COMPLETE, verbatim argv of the pane: the URL and the'; \
		echo '# target are not appended to it. Without a key the default is used'; \
		echo '# (tuicr, hunk, opencode).'; \
		echo '# The diff base must be a LOCAL ref (the clone is bare: main, not origin/main).'; \
		echo '[commands]'; \
		echo '# hunk = "hunk diff main...HEAD --watch"'; \
		echo '# tuicr = "tuicr pr"'; \
		echo '# agent = "opencode"'; \
	} > "$(CONFDIR)/config.toml"; \
	echo "created: $(CONFDIR)/config.toml"
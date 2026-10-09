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

# Mutation gate scope, read by scripts/mutate.sh (a second hardcoded copy is one more
# thing that can drift from what `make mutate` actually runs). The alternation MUST stay
# quoted where it expands: unquoted, sh reads each '|' as a pipe.
MUTATE_EXCLUDE ?= (\.worktrees/|internal/testutil/)

.DEFAULT_GOAL := help

.PHONY: help build install uninstall run print test fmt fmt-check vet lint check tidy clean config config-path mutate mutate-diff coverage coverage-check

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

# Mutation. scripts/mutate.sh owns the warm-up, the coefficient, the supervisor and the
# verdict: local and CI measure through the same path. The per-mutant deadline derives
# from ceil(cap / coverage pass); an expired mutant is absent from the totals and its
# ceiling is judged against .mutation-timeouts inside the script.
mutate: ## Whole-module mutation run, with the verdict (same wiring as CI)
	@scripts/mutate.sh --run

mutate-diff: ## Mutation run over the diff vs MUTATE_BASE, with the verdict
	@scripts/mutate.sh --diff

COVER_PROFILE ?= coverage.out

coverage: ## Coverage profile of the whole suite
	@go test -count=1 -covermode=atomic -coverprofile=$(COVER_PROFILE) ./... > /dev/null
	@echo "profile: $(COVER_PROFILE)"

coverage-check: coverage ## Gate: this change's DIFF at 100%, and the total against scripts/coverage-floor
	@scripts/diff-coverage.sh "$(COVER_PROFILE)" "$(MUTATE_BASE)"

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
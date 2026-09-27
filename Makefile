# prdash — tareas de desarrollo, instalación y prueba.
# Requiere Go 1.26+ y, para datos reales, `gh`/`glab` autenticados.

BINARY  := prdash
PKG     := ./cmd/prdash
BINDIR  ?= $(HOME)/.local/bin
CONFDIR ?= $(HOME)/.config/prdash
# Misma versión que usan dbx/gitdash/tsk/vroom; se ejecuta con `go run`, sin
# binario global. Sin .golangci.yml, golangci-lint aplica su set por defecto.
GOLANGCI_LINT_VERSION := v2.13.2
MUTATE_BASE ?= main

.DEFAULT_GOAL := help

.PHONY: help build install uninstall run print test fmt fmt-check vet lint check tidy clean config config-path mutate mutate-diff

help: ## Muestra las tareas disponibles
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compila el binario en ./bin/prdash
	@mkdir -p bin
	go build -o bin/$(BINARY) $(PKG)

install: build ## Instala el binario en $(BINDIR)/prdash
	@mkdir -p "$(BINDIR)"
	install -m 0755 bin/$(BINARY) "$(BINDIR)/$(BINARY)"
	@echo "instalado: $(BINDIR)/$(BINARY)"

uninstall: ## Elimina el binario instalado
	rm -f "$(BINDIR)/$(BINARY)"

run: ## Abre la TUI
	go run $(PKG)

print: ## Ejecuta el modo --print (sin TUI, para comprobar el pipeline)
	go run $(PKG) --print

test: ## Runner completo: build + vet + gofmt + test con -race
	go build ./...
	go vet ./...
	@fmt_out=$$(gofmt -l $$(git ls-files '*.go')); if [ -n "$$fmt_out" ]; then \
		echo "gofmt pendiente en:"; echo "$$fmt_out"; exit 1; \
	fi
	go test -race -count=1 ./...

fmt: ## Formatea el código
	gofmt -w .

fmt-check: ## Verifica formato gofmt sin modificar (falla si hay pendientes)
	@out="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$out" ]; then echo "gofmt pendiente en:"; echo "$$out"; exit 1; fi

vet: ## Analiza el código
	go vet ./...

lint: vet fmt-check ## go vet + gofmt + golangci-lint (versión pineada, siempre vía go run)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Gate local equivalente a CI: build + lint + test. `install` (que copia a
# ~/.local/bin) queda fuera a propósito; `make install` es un paso aparte.
check: build lint test
	@echo "check OK"

tidy: ## Sincroniza go.mod/go.sum
	go mod tidy

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

clean: ## Borra los artefactos de compilación
	rm -rf bin

config-path: ## Imprime la ruta esperada del config XDG
	@echo "$(CONFDIR)/config.toml"

config: ## Crea config.toml si no existe (requiere GITLAB_HOST=host.del.selfmanaged)
	@mkdir -p "$(CONFDIR)"
	@if [ -f "$(CONFDIR)/config.toml" ]; then \
		echo "ya existe: $(CONFDIR)/config.toml"; exit 0; \
	fi; \
	if [ -z "$(GITLAB_HOST)" ]; then \
		echo "falta GITLAB_HOST — ejemplo: make config GITLAB_HOST=gitlab.miempresa.com"; exit 1; \
	fi; \
	{ \
		echo '# prdash — config XDG (defaults si falta el fichero).'; \
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
		echo '# base REST de la instancia (glab resuelve host y base solo);'; \
		echo '# default de raíz. Si la instancia vive en subcarpeta, usa'; \
		echo '# api_base = "/git/api/v4/" o clone_base = "git" (relative URL'; \
		echo '# root del clon/web; clone_base = "/" fuerza raíz).'; \
		echo 'api_base = "/api/v4/"'; \
		echo; \
		echo '[forge.bitbucket]'; \
		echo 'enabled = false'; \
		echo; \
		echo '# Comandos de los panes del review. Si defines una clave, su valor es'; \
		echo '# el argv COMPLETO y verbatim del pane: no se le añade la URL ni el'; \
		echo '# target. Sin clave se usa el default (tuicr, hunk, opencode).'; \
		echo '# La base del diff debe ser ref LOCAL (el clon es bare: main, no origin/main).'; \
		echo '[commands]'; \
		echo '# hunk = "hunk diff main...HEAD --watch"'; \
		echo '# tuicr = "tuicr pr"'; \
		echo '# agent = "opencode"'; \
	} > "$(CONFDIR)/config.toml"; \
	echo "creado: $(CONFDIR)/config.toml"

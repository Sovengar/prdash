# prdash — tareas de desarrollo, instalación y prueba.
# Requiere Go 1.26+ y, para datos reales, `gh`/`glab` autenticados.

BINARY  := prdash
PKG     := ./cmd/prdash
BINDIR  ?= $(HOME)/.local/bin
CONFDIR ?= $(HOME)/.config/prdash

.DEFAULT_GOAL := help

.PHONY: help build install uninstall run print test fmt vet tidy clean config config-path

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

test: ## Runner completo: build + vet + test
	go build ./...
	go vet ./...
	go test ./...

fmt: ## Formatea el código
	gofmt -w .

vet: ## Analiza el código
	go vet ./...

tidy: ## Sincroniza go.mod/go.sum
	go mod tidy

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
		echo '# informativo: glab ya resuelve el subfolder de la instancia'; \
		echo 'api_base = "/git/api/v4/"'; \
		echo; \
		echo '[forge.bitbucket]'; \
		echo 'enabled = false'; \
	} > "$(CONFDIR)/config.toml"; \
	echo "creado: $(CONFDIR)/config.toml"

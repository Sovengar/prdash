// Subcomandos que consume el plugin de Herdr. El manifiesto
// (`plugin/herdr/herdr-plugin.toml`) invoca `prdash herdr <sub>` para atender el
// pane del inbox, la acción de montar review y el link handler de URLs de PR/MR.
//
// Ningún subcomando relanza la TUI dentro de otro: el pane del inbox corre la
// interfaz, y las acciones (montar/link) atienden una sola unidad de trabajo y
// terminan.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/tui"
)

// mountTimeout acota un montaje lanzado desde el plugin.
const mountTimeout = 5 * time.Minute

// runHerdr despacha los subcomandos del plugin y devuelve el código de salida.
func runHerdr(cfg config.Config, adapters []forge.Adapter, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "prdash herdr: falta subcomando (inbox|mount|link)")
		return 2
	}
	switch args[0] {
	case "inbox":
		return runHerdrInbox(cfg, adapters)
	case "mount":
		return runHerdrMount(cfg, args[1:])
	case "link":
		return runHerdrLink(cfg)
	default:
		fmt.Fprintf(os.Stderr, "prdash herdr: subcomando desconocido %q\n", args[0])
		return 2
	}
}

// runHerdrInbox abre la TUI en el pane que declara el manifiesto.
func runHerdrInbox(cfg config.Config, adapters []forge.Adapter) int {
	model := tui.New(cfg, adapters)
	model.SetMounter(buildExecutor(cfg))
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "prdash:", err)
		return 1
	}
	return 0
}

// runHerdrMount monta el review de la URL recibida por argumento o por el
// contexto del plugin (action/keybinding global).
func runHerdrMount(cfg config.Config, args []string) int {
	target, ok := reviewTarget(args, os.Getenv)
	if !ok {
		fmt.Fprintln(os.Stderr, "prdash herdr mount: falta la URL del PR/MR (argumento o HERDR_PLUGIN_CLICKED_URL)")
		return 1
	}
	return mountReview(cfg, target)
}

// runHerdrLink monta el review de la URL clicada que Herdr entrega al link
// handler (Ctrl+click).
func runHerdrLink(cfg config.Config) int {
	target, ok := reviewTarget(nil, os.Getenv)
	if !ok {
		fmt.Fprintln(os.Stderr, "prdash herdr link: el contexto no trae una URL de PR/MR clicada")
		return 1
	}
	return mountReview(cfg, target)
}

// mountReview resuelve la URL, monta el review y reporta el resultado.
func mountReview(cfg config.Config, target string) int {
	it, ok := reviewItem(target, hostsOf(cfg))
	if !ok {
		fmt.Fprintf(os.Stderr, "prdash herdr: %q no es una URL de PR/MR reconocida\n", target)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), mountTimeout)
	defer cancel()
	res, err := buildExecutor(cfg).Mount(ctx, it)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prdash herdr: no se pudo montar review:", err)
		return 1
	}

	if res.Herdr {
		fmt.Printf("review montado: %d panes en %s\n", len(res.Plan.Panes), res.Worktree.Path)
	} else {
		fmt.Printf("worktree montado en %s; el layout de review requiere Herdr\n", res.Worktree.Path)
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "prdash:", w)
	}
	return 0
}

// reviewTarget resuelve la URL de review de un subcomando: el primer argumento
// no vacío, o `HERDR_PLUGIN_CLICKED_URL` (env directa o del contexto JSON del
// plugin). Es lo único fiable de un link handler: no se usa selected_text.
func reviewTarget(args []string, getenv func(string) string) (string, bool) {
	for _, a := range args {
		if a = strings.TrimSpace(a); a != "" {
			return a, true
		}
	}
	if u := strings.TrimSpace(getenv("HERDR_PLUGIN_CLICKED_URL")); u != "" {
		return u, true
	}
	if u := clickedURLFromContext(getenv("HERDR_PLUGIN_CONTEXT_JSON")); u != "" {
		return u, true
	}
	return "", false
}

// clickedURLFromContext extrae clicked_url del contexto JSON del plugin. Un
// contexto ausente o ilegible no es un error: no hay URL.
func clickedURLFromContext(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var ctx struct {
		ClickedURL string `json:"clicked_url"`
	}
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return ""
	}
	return strings.TrimSpace(ctx.ClickedURL)
}

// reviewItem traduce una URL de PR/MR al ítem mínimo que el montaje necesita
// (forge, host, proyecto, número y URL). El fetch y la rama los resuelve el
// resolutor a partir del ref de review.
func reviewItem(target string, hosts map[string]string) (model.Item, bool) {
	ref, number, ok := parse.ParseReviewURL(target, hosts)
	if !ok {
		return model.Item{}, false
	}
	it := model.NewItem(ref, number)
	it.URL = target
	return it, true
}

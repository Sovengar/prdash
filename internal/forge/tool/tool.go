package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const DefaultTimeout = time.Duration(30e9)

const pipeCloseGrace = time.Duration(250e6)

type Runner struct {
	Bin     string
	Timeout time.Duration
	Extra   []string // extra forge env vars (e.g. GH_PROMPT_DISABLED=1)
}

func New(bin string, extra ...string) *Runner {
	return &Runner{Bin: bin, Timeout: DefaultTimeout, Extra: extra}
}

type Error struct {
	Bin      string
	Args     []string
	ExitCode int
	Msg      string
	Err      error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("%s %s: %s", e.Bin, strings.Join(e.Args, " "), e.Msg)
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

func (r *Runner) Run(ctx context.Context, args ...string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, r.Bin, args...)
	cmd.Env = Env(r.Extra...)
	cmd.WaitDelay = pipeCloseGrace
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := FirstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		cerr := &Error{Bin: r.Bin, Args: args, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			cerr.ExitCode = exit.ExitCode()
		}
		return out.String(), cerr
	}
	return out.String(), nil
}

func Env(extra ...string) []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1")
	return append(out, extra...)
}

func ExitCode(err error) int {
	var cerr *Error
	if errors.As(err, &cerr) {
		return cerr.ExitCode
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 0
}

var (
	httpCodeRe   = regexp.MustCompile(`(?i)\bhttp(?:/\d(?:\.\d)?)?\s+(\d{3})\b`)
	statusCodeRe = regexp.MustCompile(`(?i)\bstatus(?:\s+code)?[:\s]+(\d{3})\b`)
)

func HTTPStatus(msg string) int {
	for _, re := range []*regexp.Regexp{httpCodeRe, statusCodeRe} {
		if m := re.FindStringSubmatch(msg); m != nil {
			code := 0
			for _, r := range m[1] {
				code = code*10 + int(r-'0')
			}
			return code
		}
	}
	return 0
}

func Kind(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())

	if isRateLimitText(msg) {
		return "ratelimit"
	}
	if isUnmergeableText(msg) {
		return "unmergeable"
	}
	if isSelfReviewText(msg) {
		return "selfreview"
	}
	if code := HTTPStatus(err.Error()); code != 0 {
		if k := kindForHTTP(code); k != "" {
			return k
		}
	}

	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "429"), strings.Contains(msg, "abuse"):
		return "ratelimit"
	case strings.Contains(msg, "409"), strings.Contains(msg, "conflict"),
		strings.Contains(msg, "already closed"), strings.Contains(msg, "already merged"):
		return "conflict"
		// Its own class: well-formed request, bad content — a refresh does not help and it is not a permission.
	case strings.Contains(msg, "422"):
		return "validation"
	case strings.Contains(msg, "401"), strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "not logged"), strings.Contains(msg, "authentication failed"):
		return "auth"
	case strings.Contains(msg, "403"), strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "not authorized"), strings.Contains(msg, "insufficient"),
		strings.Contains(msg, "permission"), strings.Contains(msg, "must have push"):
		return "permission"
	case strings.Contains(msg, "404"), strings.Contains(msg, "not found"):
		return "notfound"
	default:
		return "network"
	}
}

func isUnmergeableText(lower string) bool {
	return strings.Contains(lower, "not mergeable") ||
		strings.Contains(lower, "cannot be cleanly created") ||
		strings.Contains(lower, "head branch was modified") ||
		strings.Contains(lower, "cannot merge") ||
		strings.Contains(lower, "cannot be merged") ||
		strings.Contains(lower, "need to rebase") ||
		strings.Contains(lower, "not up to date")
}

func isSelfReviewText(lower string) bool {
	return strings.Contains(lower, "approve your own")
}

func isRateLimitText(lower string) bool {
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "abuse") ||
		strings.Contains(lower, "too many requests")
}

func kindForHTTP(code int) string {
	if code == 401 {
		return "auth"
	}
	if code == 403 {
		return "permission"
	}
	if code == 404 {
		return "notfound"
	}
	if code == 409 {
		return "conflict"
	}
	if code == 422 {
		return "validation"
	}
	if code == 429 {
		return "ratelimit"
	}
	if code >= 500 {
		return "network"
	}
	return ""
}

func APIMessage(body string) string {
	var payload struct {
		Message any `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	for _, e := range payload.Errors {
		if msg := strings.TrimSpace(e.Message); msg != "" {
			return msg
		}
	}
	if msg, ok := payload.Message.(string); ok {
		return strings.TrimSpace(msg)
	}
	return ""
}

func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

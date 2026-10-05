// The "mount review" action in the TUI, with fake mounters.
package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge/model"
	"prdash/internal/review/executor"
	"prdash/internal/worktree"
)

type fakeMounter struct {
	res executor.Result
	err error
	got model.Item
}

func (f *fakeMounter) Mount(_ context.Context, it model.Item) (executor.Result, error) {
	f.got = it
	return f.res, f.err
}

func mountModel(t *testing.T) (Model, model.Item) {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "Revisar widget", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	return showSection(m, model.SectionAuthored), it
}

func waitMount(t *testing.T, m Model) tea.Msg {
	t.Helper()
	select {
	case ev := <-m.events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("the mount result never arrived")
		return nil
	}
}

func TestMountReviewWithoutMounterInformsHerdrRequired(t *testing.T) {
	m, _ := mountModel(t)

	m = press(t, m, "r")
	if !strings.Contains(lastToast(m), "requires Herdr") {
		t.Fatalf("toast = %q, wants it to mention Herdr", lastToast(m))
	}
	if m.mountBusy {
		t.Fatal("with no mounter there should be no mount in flight")
	}
}

func TestMountReviewOutsideHerdrReportsLayoutUnavailable(t *testing.T) {
	m, it := mountModel(t)
	fm := &fakeMounter{res: executor.Result{
		Worktree: worktree.Worktree{Path: "/tmp/wt/prdash-pr-1", Branch: "prdash/pr-1"},
		Herdr:    false,
	}}
	m.SetMounter(fm)

	m = press(t, m, "r")
	if !m.mountBusy {
		t.Fatal("the mount should be marked as in flight")
	}

	m = send(t, m, waitMount(t, m))
	if !strings.Contains(lastToast(m), "requires Herdr") {
		t.Fatalf("toast = %q, wants the layout-unavailable toast", lastToast(m))
	}
	if m.mountBusy {
		t.Fatal("the finished mount should not stay marked as in flight")
	}
	if fm.got.ID() != it.ID() {
		t.Fatalf("mounted %+v, want %+v", fm.got.ID(), it.ID())
	}
}

func TestMountReviewErrorSurfacesNotice(t *testing.T) {
	m, _ := mountModel(t)
	m.SetMounter(&fakeMounter{err: errors.New("no fetch permissions")})

	m = press(t, m, "r")
	m = send(t, m, waitMount(t, m))

	if !strings.Contains(lastToast(m), "could not mount review") {
		t.Fatalf("toast = %q, wants the mount error", lastToast(m))
	}
	if !strings.Contains(lastToast(m), "no fetch permissions") {
		t.Fatalf("toast = %q, wants the cause", lastToast(m))
	}
}

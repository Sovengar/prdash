package main

import (
	"context"
	"fmt"
	"os"

	"prdash/internal/herdr"
	"prdash/internal/worktree"
)

func main() {
	ctx := context.Background()
	c := herdr.New()
	fmt.Println("HERDR_ENV =", os.Getenv("HERDR_ENV"))
	v, ok := c.Version()
	fmt.Printf("Version = %v (parsed=%v)  Available = %v\n", v, ok, c.Available())

	const repo = "/home/buble/dev/projects/prdash"
	const dest = "/home/buble/.local/share/prdash/worktrees/github/github.com/Sovengar/prdash/prdash-pr-13"

	p := worktree.Select(c, os.Getenv("HOME")+"/.local/share/prdash/worktrees")
	fmt.Printf("provisioner = %T\n", p)
	fmt.Printf("Exists(%s) = %v\n", dest, worktree.Exists(dest))

	wt, err := p.Create(ctx, worktree.Spec{Repo: repo, Branch: "prdash/pr-13", Path: dest, Label: "prdash-pr-13"})
	fmt.Printf("Create err = %v\n", err)
	fmt.Printf("worktree = %+v\n", wt)
}

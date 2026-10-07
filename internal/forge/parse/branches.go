package parse

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ParseGHBranches(out string) []string {
	return branchLines(out)
}

func ParseGLBranches(out string) ([]string, error) {
	var names []string
	var lastErr error
	seen := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		seen++
		var br struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(line), &br); err != nil {
			lastErr = err
			continue
		}
		if br.Name != "" {
			names = append(names, br.Name)
		}
	}
	if seen > 0 && len(names) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("could not read the branch list: %w", lastErr)
		}
		return nil, fmt.Errorf("could not read the branch list")
	}
	return names, nil
}

func branchLines(out string) []string {
	raw := strings.Split(out, "\n")
	names := make([]string, 0, len(raw))
	for _, line := range raw {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

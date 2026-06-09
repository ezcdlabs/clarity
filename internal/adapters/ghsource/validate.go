package ghsource

import (
	"context"
	"fmt"
	"strings"
)

// Validate is a fast pre-flight check: it confirms gh is reachable and
// each configured workflow name actually exists on the repo. Designed
// to be called once at CLI startup so errors surface visibly before
// the TUI claims the terminal — the alternative (silently degrading
// to an empty snapshot) gives users no signal that anything's wrong.
//
// Currently ignores ctx — the call delegates to ListWorkflows, whose
// CLI shell-out doesn't accept cancellation. The parameter is there so
// future on-the-wire clients (or a Validate that does a Run + Jobs
// roundtrip) can honour ctx without changing the signature.
func (s *Source) Validate(_ context.Context) error {
	if s.opts.Mapping == nil {
		return fmt.Errorf("no clarity.github mapping configured")
	}
	configured := s.configuredWorkflowNames()
	if len(configured) == 0 {
		return fmt.Errorf("clarity.github has neither ci nor deploy populated")
	}

	available, err := s.opts.Client.ListWorkflows()
	if err != nil {
		return fmt.Errorf("list workflows (check `gh auth status` and network): %w", err)
	}
	if len(available) == 0 {
		return fmt.Errorf("no workflows defined on the repository — clarity.github needs at least one .github/workflows/*.yml to map against")
	}

	availableNames := make([]string, 0, len(available))
	availableSet := make(map[string]bool, len(available))
	for _, w := range available {
		availableNames = append(availableNames, w.Name)
		availableSet[w.Name] = true
	}
	var missing []string
	for _, name := range configured {
		if !availableSet[name] {
			missing = append(missing, fmt.Sprintf("%q", name))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("workflow(s) not found on the repository: %s\navailable workflows: %s",
			strings.Join(missing, ", "),
			strings.Join(availableNames, ", "))
	}
	return nil
}

// configuredWorkflowNames returns the distinct workflow names referenced
// by the mapping. Deduped so a config that points both CI and deploy at
// the same workflow doesn't double-up in error messages.
func (s *Source) configuredWorkflowNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	if s.opts.Mapping.CI != nil {
		add(s.opts.Mapping.CI.Workflow)
	}
	if s.opts.Mapping.Deploy != nil {
		add(s.opts.Mapping.Deploy.Workflow)
	}
	return out
}

package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ezcdlabs/clarity/internal/core"
)

// rawDeploy is one entry of the `deploys` list. The key accepts two shapes —
// a bare string, or an object — so the common case (a flow whose name is its
// only target) stays a one-liner while a renamed flow can still spell out the
// target set it owns.
type rawDeploy struct {
	Name    string
	Targets []string
	// explicit records whether the entry listed `targets` at all, so an
	// object form with the key omitted can default to its own name rather
	// than to "claims nothing".
	explicit bool
}

func (d *rawDeploy) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		d.Name, d.Targets, d.explicit = name, nil, false
		return nil
	}
	var obj struct {
		Name    string    `json:"name"`
		Targets *[]string `json:"targets"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		// Name the likely mistake rather than echoing the decoder. The
		// common typo is a singular `"targets": "web"`, and telling someone
		// their object is not an object sends them looking in the wrong place.
		if strings.Contains(err.Error(), "field .targets") {
			return fmt.Errorf(`clarity.deploys: "targets" must be a list of target names, e.g. ["web"]`)
		}
		return fmt.Errorf(`clarity.deploys: entry must be a flow name or an object with "name" and optional "targets"`)
	}
	d.Name = obj.Name
	if obj.Targets != nil {
		d.Targets, d.explicit = *obj.Targets, true
	} else {
		d.Targets, d.explicit = []string{obj.Name}, false
	}
	return nil
}

// Deploys returns the declared deploy flows in declaration order, which is
// also their display order. Nil when the file declares none — that is the
// zero-setup path, where flows are discovered from the events instead and
// nothing is ever hidden.
func (c Config) Deploys() []core.Flow {
	if c.Clarity == nil {
		return nil
	}
	return c.Clarity.Deploys
}

// hydrateDeploys converts the wire entries into flows and rejects the
// ambiguities that would otherwise have to be resolved arbitrarily later:
// an event or a --deploy argument must map to exactly one flow, always.
func hydrateDeploys(raw []rawDeploy) ([]core.Flow, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	flows := make([]core.Flow, 0, len(raw))
	names := map[string]bool{}
	owners := map[string]string{}

	for i, d := range raw {
		where := fmt.Sprintf("clarity.deploys[%d]", i)

		if strings.TrimSpace(d.Name) == "" {
			return nil, fmt.Errorf("%s: every entry needs a name", where)
		}
		key := core.FoldName(d.Name)
		if names[key] {
			return nil, fmt.Errorf("%s: %q is declared twice", where, d.Name)
		}
		names[key] = true

		targets := d.Targets
		if !d.explicit {
			targets = []string{d.Name}
		} else if len(targets) == 0 {
			// Every other route to "no targets given" defaults to the flow's
			// own name. An explicit empty list diverging silently would leave
			// a flow that claims nothing, renders as a permanently failed
			// expectation, and watches its real deploys surface beside it as
			// an undeclared flow.
			return nil, fmt.Errorf(`%s: %q lists no targets — omit "targets" for it to claim its own name`, where, d.Name)
		}

		mine := map[string]bool{}
		for _, t := range targets {
			// Targets are compared under the same fold as names. Accepting
			// "Web" and "web" as distinct would make --deploy=web resolve to
			// one flow while the deploy events sat in the other, which is the
			// silent wrong-subsystem outcome the matcher exists to avoid.
			key := core.FoldName(t)
			if mine[key] {
				return nil, fmt.Errorf("%s: %q lists %s twice", where, d.Name, describeTarget(t))
			}
			mine[key] = true
			if prev, taken := owners[key]; taken {
				return nil, fmt.Errorf("%s: %s is claimed by both %q and %q",
					where, describeTarget(t), prev, d.Name)
			}
			owners[key] = d.Name
		}
		flows = append(flows, core.Flow{Name: d.Name, Targets: targets})
	}

	// A flow's name must not be another flow's target: `--deploy=web` has to
	// mean one thing, and matching falls back from names to targets. Compared
	// case-insensitively, because that is how the flag will match — a config
	// accepted now must not start failing when the flag lands.
	for _, f := range flows {
		if owner, taken := owners[core.FoldName(f.Name)]; taken && owner != f.Name {
			return nil, fmt.Errorf("clarity.deploys: %q is a flow name and also a target of %q", f.Name, owner)
		}
	}
	return flows, nil
}

func describeTarget(t string) string {
	if t == "" {
		return "the untargeted deploy"
	}
	return fmt.Sprintf("target %q", t)
}

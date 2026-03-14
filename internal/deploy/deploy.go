package deploy

import "fmt"

// Target describes a documentation deployment target configured in task-plus.yml.
type Target struct {
	Type   string `yaml:"type"`
	Site   string `yaml:"site"`    // statichost site name
	RCSite string `yaml:"rc_site"` // optional RC site for pre-release verification
}

// HasRCSite returns true if this target has an RC site configured.
func (t Target) HasRCSite() bool {
	return t.RCSite != ""
}

// Deployer deploys documentation to a hosting provider.
type Deployer interface {
	Name() string
	Deploy(projectDir, docsDir string, dryRun bool) error
}

// New creates a Deployer for the given target configuration.
func New(t Target) (Deployer, error) {
	switch t.Type {
	case "github":
		return &GitHub{}, nil
	case "statichost":
		if t.Site == "" {
			return nil, fmt.Errorf("statichost deploy requires 'site' field")
		}
		return &Statichost{Site: t.Site}, nil
	default:
		return nil, fmt.Errorf("unknown deploy type: %q (supported: github, statichost)", t.Type)
	}
}

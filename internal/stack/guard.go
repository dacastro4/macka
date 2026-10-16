package stack

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Project is a Compose project known to the Docker host.
type Project struct {
	Name        string
	ConfigFiles []string
}

// ProjectCollisionError means another stack directory owns the project name.
type ProjectCollisionError struct {
	Name  string
	Owner string
}

func (e *ProjectCollisionError) Error() string {
	return fmt.Sprintf("docker project %q is already used by %s; pick another project name in stack.yml", e.Name, e.Owner)
}

// GuardProject refuses to apply when a different stack directory already owns
// the project name, so two installs never recreate each other's containers (D-11).
func GuardProject(existing []Project, name, composePath string) error {
	want := filepath.Clean(composePath)
	for _, p := range existing {
		if p.Name != name {
			continue
		}
		for _, f := range p.ConfigFiles {
			if filepath.Clean(strings.TrimSpace(f)) == want {
				return nil
			}
		}
		return &ProjectCollisionError{Name: name, Owner: strings.Join(p.ConfigFiles, ", ")}
	}
	return nil
}

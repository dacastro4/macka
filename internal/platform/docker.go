// Package platform isolates OS and Docker commands from core logic (D-10).
package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dacastro4/macka/internal/stack"
)

// Compose drives the `docker compose` CLI. Only JSON output is parsed.
type Compose struct {
	// Bin is the docker binary; defaults to "docker".
	Bin string
}

func (c Compose) bin() string {
	if c.Bin == "" {
		return "docker"
	}
	return c.Bin
}

func (c Compose) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Projects lists Compose projects on this host, including stopped ones.
func (c Compose) Projects(ctx context.Context) ([]stack.Project, error) {
	out, err := c.run(ctx, "compose", "ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return ParseProjects(out)
}

// ParseProjects decodes `docker compose ls --format json`
// ([{"Name","Status","ConfigFiles":"a.yml,b.yml"}]).
func ParseProjects(out []byte) ([]stack.Project, error) {
	var raw []struct {
		Name        string `json:"Name"`
		ConfigFiles string `json:"ConfigFiles"`
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse docker compose ls: %w", err)
	}
	ps := make([]stack.Project, 0, len(raw))
	for _, r := range raw {
		ps = append(ps, stack.Project{Name: r.Name, ConfigFiles: strings.Split(r.ConfigFiles, ",")})
	}
	return ps, nil
}

// Up starts or updates the project. Unchanged services are left running.
func (c Compose) Up(ctx context.Context, s *stack.State) error {
	_, err := c.run(ctx, "compose", "--project-directory", s.Dir, "-f", s.ComposePath(), "-p", s.Project,
		"up", "-d", "--remove-orphans", "--quiet-pull")
	return err
}

// Container is one container in a project.
type Container struct {
	ID        string `json:"ID"`
	Service   string `json:"Service"`
	State     string `json:"State"`
	CreatedAt string `json:"CreatedAt"`
}

// Containers lists the project's containers.
func (c Compose) Containers(ctx context.Context, project string) ([]Container, error) {
	out, err := c.run(ctx, "compose", "-p", project, "ps", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	return ParseContainers(out)
}

// ParseContainers accepts both a JSON array and one JSON object per line
// (Compose v5.6.0 prints the latter; COMPATIBILITY.md).
func ParseContainers(out []byte) ([]Container, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	if out[0] == '[' {
		var cs []Container
		if err := json.Unmarshal(out, &cs); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		return cs, nil
	}
	var cs []Container
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var c Container
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		cs = append(cs, c)
	}
	return cs, sc.Err()
}

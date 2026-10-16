package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hli/internal/stack"
)

func TestParseProjects(t *testing.T) {
	// Shape recorded from Compose v5.6.0 (COMPATIBILITY.md).
	ps, err := ParseProjects([]byte(`[{"Name":"lsprobe","Status":"running(1)","ConfigFiles":"/tmp/lsprobe/c.yml,/tmp/lsprobe/o.yml"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "lsprobe" || len(ps[0].ConfigFiles) != 2 {
		t.Fatalf("%+v", ps)
	}
	if ps, err := ParseProjects([]byte("  \n")); err != nil || ps != nil {
		t.Fatalf("empty output: %v %v", ps, err)
	}
}

func TestParseContainersBothShapes(t *testing.T) {
	lines := []byte(`{"ID":"5e0845b1737a","Service":"a","State":"running","CreatedAt":"2026-10-15 19:31:09 -0400 EDT"}
{"ID":"6f0845b1737b","Service":"b","State":"exited","CreatedAt":"2026-10-15 19:31:10 -0400 EDT"}`)
	cs, err := ParseContainers(lines)
	if err != nil || len(cs) != 2 || cs[1].Service != "b" {
		t.Fatalf("ndjson: %+v %v", cs, err)
	}
	cs, err = ParseContainers([]byte(`[{"ID":"x","Service":"a","State":"running"}]`))
	if err != nil || len(cs) != 1 || cs[0].ID != "x" {
		t.Fatalf("array: %+v %v", cs, err)
	}
}

// fakeDocker writes a stand-in docker binary that records its arguments and
// prints canned JSON, so the exec paths run without Docker.
func fakeDocker(t *testing.T, script string) (Compose, string) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "docker")
	body := "#!/bin/sh\necho \"$@\" >> " + argsFile + "\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return Compose{Bin: bin}, argsFile
}

func TestComposeCommands(t *testing.T) {
	c, argsFile := fakeDocker(t, `case "$*" in
  *" ls "*) echo '[{"Name":"hli","ConfigFiles":"/s/docker-compose.yml"}]' ;;
  *" ps "*) echo '{"ID":"a1","Service":"radarr","State":"running"}' ;;
esac`)
	ctx := context.Background()
	ps, err := c.Projects(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "hli" {
		t.Fatalf("projects %+v %v", ps, err)
	}
	cs, err := c.Containers(ctx, "hli")
	if err != nil || len(cs) != 1 || cs[0].Service != "radarr" {
		t.Fatalf("containers %+v %v", cs, err)
	}
	s := &stack.State{Dir: "/s", Project: "hli"}
	if err := c.Up(ctx, s); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(raw), "compose --project-directory /s -f /s/docker-compose.yml -p hli up -d --remove-orphans --quiet-pull") {
		t.Fatalf("up args:\n%s", raw)
	}
}

func TestComposeErrorIncludesStderr(t *testing.T) {
	c, _ := fakeDocker(t, `echo "permission denied while trying to connect" >&2; exit 1`)
	_, err := c.Projects(context.Background())
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want stderr in error, got %v", err)
	}
}

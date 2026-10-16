package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hli/internal/compat"
	"hli/internal/redact"
	"hli/internal/stack"
)

type fakeCompose struct {
	projects []stack.Project
	ups      int
}

func (f *fakeCompose) Projects(context.Context) ([]stack.Project, error) { return f.projects, nil }
func (f *fakeCompose) Up(context.Context, *stack.State) error            { f.ups++; return nil }

const stackYML = `version: 1
project: hli
timezone: Etc/UTC
bindAddress: 127.0.0.1
paths: {data: ./data}
apps: [prowlarr, radarr, sonarr, jellyfin]
`

func writeStack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "stack.yml")
	if err := os.WriteFile(p, []byte(stackYML), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newDeps(fc *fakeCompose, pins compat.Pins) (deps, *bytes.Buffer) {
	var out bytes.Buffer
	return deps{compose: fc, pins: pins, stdout: redact.NewWriter(&out), stderr: redact.NewWriter(&out)}, &out
}

// leakyServer answers readiness probes, then fails every call with an error
// body that echoes the credentials it received, the worst case for logging.
func leakyServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			return
		}
		if r.URL.Path == "/System/Info/Public" {
			_ = json.NewEncoder(w).Encode(map[string]any{"StartupWizardCompleted": false})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/Startup/") && !(r.Method == http.MethodPost && r.URL.Path == "/Startup/User") {
			_, _ = w.Write([]byte("{}"))
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request key=" + r.Header.Get("X-Api-Key") + " body=" + string(body)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestT5_SecretsNotLogged(t *testing.T) {
	srv := leakyServer(t)
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	pins := compat.Pins{}
	for k, v := range compat.Default {
		v.Port = port
		pins[k] = v
	}
	path := writeStack(t)
	d, out := newDeps(&fakeCompose{}, pins)

	code := run(context.Background(), []string{"apply", "-f", path}, d)
	if code != 1 {
		t.Fatalf("want exit 1 when links fail, got %d\n%s", code, out)
	}
	s, _ := stack.Load(path)
	sec, err := stack.LoadOrCreateSecrets(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), redact.Mask) {
		t.Fatalf("expected the echoed secrets to be masked; output:\n%s", out)
	}
	if !strings.Contains(out.String(), "Jellyfin admin account: FAILED") {
		t.Fatalf("jellyfin setup should fail echoing the password; output:\n%s", out)
	}
	for _, v := range sec.Values() {
		if strings.Contains(out.String(), v) {
			t.Fatalf("secret leaked into output:\n%s", out)
		}
	}
}

func TestT2_ApplyInvalidStateChangesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.yml")
	_ = os.WriteFile(path, []byte(strings.Replace(stackYML, "127.0.0.1", "0.0.0.0", 1)), 0o644)
	fc := &fakeCompose{}
	d, out := newDeps(fc, compat.Default)
	if code := run(context.Background(), []string{"apply", "-f", path}, d); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "bindAddress") {
		t.Fatalf("error must name the field: %s", out)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || fc.ups != 0 {
		t.Fatalf("invalid state changed something: %d entries, %d ups", len(entries), fc.ups)
	}
}

func TestD11_ApplyRefusesCollisionWithoutChanges(t *testing.T) {
	path := writeStack(t)
	fc := &fakeCompose{projects: []stack.Project{{Name: "hli", ConfigFiles: []string{"/elsewhere/docker-compose.yml"}}}}
	d, out := newDeps(fc, compat.Default)
	if code := run(context.Background(), []string{"apply", "-f", path}, d); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "already used by /elsewhere") {
		t.Fatalf("output: %s", out)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 || fc.ups != 0 {
		t.Fatalf("collision must change nothing: %d entries, %d ups", len(entries), fc.ups)
	}
}

func TestStatusBeforeApply(t *testing.T) {
	d, out := newDeps(&fakeCompose{}, compat.Default)
	if code := run(context.Background(), []string{"status", "-f", writeStack(t)}, d); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "not been applied") {
		t.Fatalf("output: %s", out)
	}
}

func TestUsage(t *testing.T) {
	d, _ := newDeps(&fakeCompose{}, compat.Default)
	if code := run(context.Background(), nil, d); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if code := run(context.Background(), []string{"bogus"}, d); code != 2 {
		t.Fatalf("exit %d", code)
	}
}

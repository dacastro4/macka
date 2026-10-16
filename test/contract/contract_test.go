//go:build contract

// Package contract runs macka against real pinned containers (CC-3). It needs a
// Linux host with Docker and nothing else bound to the app ports.
//
//	MACKA_BIN=/path/to/macka go test -tags contract ./test/contract
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const stackYML = `version: 1
project: %s
timezone: Etc/UTC
bindAddress: 127.0.0.1
paths: {data: ./data}
apps: [prowlarr, radarr, sonarr, jellyfin]
`

type env struct {
	bin, dir, project string
}

func setup(t *testing.T) env {
	t.Helper()
	bin := os.Getenv("MACKA_BIN")
	if bin == "" {
		t.Skip("MACKA_BIN not set")
	}
	dir := t.TempDir()
	project := fmt.Sprintf("mackact%d", time.Now().Unix())
	if err := os.WriteFile(filepath.Join(dir, "stack.yml"), []byte(fmt.Sprintf(stackYML, project)), 0o644); err != nil {
		t.Fatal(err)
	}
	e := env{bin: bin, dir: dir, project: project}
	t.Cleanup(func() {
		if os.Getenv("MACKA_KEEP") != "" {
			return
		}
		cmd := exec.Command("docker", "compose", "-p", project, "down", "-v")
		cmd.Dir = dir
		_ = cmd.Run()
		// Containers write config as the stack owner; remove with docker in case
		// any file was created by a container-internal user.
		_ = exec.Command("docker", "run", "--rm", "-v", dir+":/d", "busybox:1.37", "sh", "-c", "rm -rf /d/*").Run()
	})
	return e
}

func (e env) macka(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(e.bin, append(args, "-f", filepath.Join(e.dir, "stack.yml"))...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), code
}

func (e env) containers(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "compose", "-p", e.project, "ps", "-a", "--format", "{{.ID}} {{.Service}} {{.CreatedAt}} {{.State}}").Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func (e env) secrets(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(e.dir, "secrets", "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

var links = []string{
	"Radarr media folder /data/media/movies",
	"Sonarr media folder /data/media/tv",
	"Prowlarr to Radarr",
	"Prowlarr to Sonarr",
	"Jellyfin admin account",
	"Jellyfin API key for macka",
	"Jellyfin Movies library",
	"Jellyfin Shows library",
	"Radarr to Jellyfin (library refresh)",
	"Sonarr to Jellyfin (library refresh)",
}

// TestP4_GateTwoRuns is the M0 gate: a fresh host reaches a fully wired stack,
// and a second run changes nothing (core-video-stack: M0 gate).
func TestP4_GateTwoRuns(t *testing.T) {
	e := setup(t)

	out, code := e.macka(t, "apply")
	t.Logf("first run:\n%s", out)
	if code != 0 {
		t.Fatalf("first apply exit %d", code)
	}
	for _, l := range links {
		if !strings.Contains(out, l+": connected") {
			t.Errorf("first run missing %q", l)
		}
	}
	if !strings.Contains(out, "10 applied, 0 already satisfied, 0 not connected") {
		t.Errorf("first run summary wrong")
	}
	before := e.containers(t)

	out, code = e.macka(t, "apply")
	t.Logf("second run:\n%s", out)
	if code != 0 {
		t.Fatalf("second apply exit %d", code)
	}
	if !strings.Contains(out, "0 applied, 10 already satisfied, 0 not connected") {
		t.Errorf("second run must change nothing")
	}
	if after := e.containers(t); after != before {
		t.Errorf("containers recreated:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	out, code = e.macka(t, "status")
	if code != 0 || !strings.Contains(out, "10 already satisfied") {
		t.Errorf("status after apply: exit %d\n%s", code, out)
	}

	t.Run("no default credentials", func(t *testing.T) { checkCredentials(t, e) })
}

func checkCredentials(t *testing.T, e env) {
	sec := e.secrets(t)
	keys := sec["apiKeys"].(map[string]any)
	for app, port := range map[string]string{"radarr": "7878/api/v3", "sonarr": "8989/api/v3", "prowlarr": "9696/api/v1"} {
		url := "http://127.0.0.1:" + port + "/system/status"
		if code := status(t, "GET", url, "", nil); code != 401 {
			t.Errorf("%s without key: HTTP %d, want 401", app, code)
		}
		if code := status(t, "GET", url, "", map[string]string{"X-Api-Key": keys[app].(string)}); code != 200 {
			t.Errorf("%s with generated key: HTTP %d", app, code)
		}
	}
	auth := map[string]string{"Authorization": `MediaBrowser Client="macka-contract", Device="ct", DeviceId="ct", Version="0"`}
	login := "http://127.0.0.1:8096/Users/AuthenticateByName"
	for _, pw := range []string{"", "admin", "password"} {
		body := fmt.Sprintf(`{"Username":"admin","Pw":%q}`, pw)
		if code := status(t, "POST", login, body, auth); code != 401 {
			t.Errorf("jellyfin accepted a default password %q: HTTP %d", pw, code)
		}
	}
	body := fmt.Sprintf(`{"Username":"admin","Pw":%q}`, sec["jellyfinAdminPassword"])
	if code := status(t, "POST", login, body, auth); code != 200 {
		t.Errorf("jellyfin generated password: HTTP %d", code)
	}
}

func status(t *testing.T, method, url, body string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

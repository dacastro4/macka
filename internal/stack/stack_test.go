package stack

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"hli/internal/compat"
)

const validYAML = `version: 1
project: hli
timezone: Etc/UTC
bindAddress: 127.0.0.1
paths:
  data: ./data
user: {uid: 1000, gid: 1000}
apps: [prowlarr, radarr, sonarr, jellyfin]
`

func mustParse(t *testing.T, raw, dir string) *State {
	t.Helper()
	s, err := Parse([]byte(raw), dir)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}

func TestT2_InvalidStateRejected(t *testing.T) {
	cases := map[string]struct {
		edit  func(string) string
		field string
	}{
		"wrong version":     {func(s string) string { return strings.Replace(s, "version: 1", "version: 2", 1) }, "version"},
		"bad project":       {func(s string) string { return strings.Replace(s, "project: hli", "project: Bad Name", 1) }, "project"},
		"public bind":       {func(s string) string { return strings.Replace(s, "127.0.0.1", "0.0.0.0", 1) }, "bindAddress"},
		"internet bind":     {func(s string) string { return strings.Replace(s, "127.0.0.1", "8.8.8.8", 1) }, "bindAddress"},
		"unknown app":       {func(s string) string { return strings.Replace(s, "jellyfin]", "plex]", 1) }, "apps"},
		"duplicate app":     {func(s string) string { return strings.Replace(s, "jellyfin]", "radarr]", 1) }, "apps"},
		"missing data path": {func(s string) string { return strings.Replace(s, "data: ./data", "data: \"\"", 1) }, "paths.data"},
		"unknown field":     {func(s string) string { return s + "extra: true\n" }, "(document)"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.edit(validYAML)), t.TempDir())
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			if ve.Field != tc.field {
				t.Fatalf("want field %q, got %q (%v)", tc.field, ve.Field, err)
			}
		})
	}
}

func TestT2_InvalidStateChangesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.yml")
	if err := os.WriteFile(path, []byte("version: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("want error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("invalid state must not create files, found %d entries", len(entries))
	}
}

func TestT2_DeterministicGeneration(t *testing.T) {
	s := mustParse(t, validYAML, "/srv/stack")
	u := User{UID: 1000, GID: 1000}
	a, err := Generate(s, compat.Default, u)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		b, err := Generate(s, compat.Default, u)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Compose, b.Compose) || !bytes.Equal(a.Env, b.Env) {
			t.Fatal("generation is not byte-identical across runs")
		}
	}
	if !bytes.HasPrefix(a.Compose, []byte(GeneratedHeader)) || !bytes.HasPrefix(a.Env, []byte(GeneratedHeader)) {
		t.Fatal("generated files must start with the do-not-edit header")
	}
}

func TestT2_GeneratedComposeIsPinnedAndLocal(t *testing.T) {
	s := mustParse(t, validYAML, "/srv/stack")
	g, err := Generate(s, compat.Default, User{UID: 1000, GID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(g.Compose, &cf); err != nil {
		t.Fatalf("generated compose is not valid YAML: %v", err)
	}
	if cf.Name != "hli" {
		t.Fatalf("project name %q", cf.Name)
	}
	for app, svc := range cf.Services {
		if !strings.Contains(svc.Image, ":") || !strings.Contains(svc.Image, "@sha256:") {
			t.Errorf("%s image not pinned by tag and digest: %s", app, svc.Image)
		}
		for _, p := range svc.Ports {
			if !strings.HasPrefix(p, "${BIND_ADDRESS}:") {
				t.Errorf("%s port %q not bound to BIND_ADDRESS (X-3)", app, p)
			}
		}
	}
	if !strings.Contains(string(g.Env), "BIND_ADDRESS=127.0.0.1\n") {
		t.Fatal(".env missing bind address")
	}
}

func TestT2_UnpinnedImageRefused(t *testing.T) {
	s := mustParse(t, validYAML, "/srv/stack")
	pins := compat.Pins{}
	for k, v := range compat.Default {
		pins[k] = v
	}
	r := pins["radarr"]
	r.Image.Digest = ""
	pins["radarr"] = r

	_, err := Generate(s, pins, User{})
	var ue *UnpinnedError
	if !errors.As(err, &ue) || ue.App != "radarr" {
		t.Fatalf("want UnpinnedError for radarr, got %v", err)
	}
	if !strings.Contains(err.Error(), "docs/COMPATIBILITY.md") {
		t.Fatal("error must point to docs/COMPATIBILITY.md")
	}
}

func TestS1_SameDataPathAllContainers(t *testing.T) {
	s := mustParse(t, validYAML, "/srv/stack")
	g, err := Generate(s, compat.Default, User{UID: 1000, GID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(g.Compose, &cf); err != nil {
		t.Fatal(err)
	}
	for _, app := range []string{"jellyfin", "radarr", "sonarr"} {
		found := 0
		for _, v := range cf.Services[app].Volumes {
			if v == "${DATA_ROOT}:/data" {
				found++
			}
		}
		if found != 1 {
			t.Errorf("%s must mount the data root exactly once at /data, volumes=%v", app, cf.Services[app].Volumes)
		}
	}
	if !strings.Contains(string(g.Env), "DATA_ROOT=/srv/stack/data\n") {
		t.Fatalf("data root not resolved against stack dir: %s", g.Env)
	}
}

func TestT5_KeyKnownBeforeFirstBoot(t *testing.T) {
	dir := t.TempDir()
	s := mustParse(t, validYAML, dir)
	u := User{UID: os.Getuid(), GID: os.Getgid()}

	sec, err := LoadOrCreateSecrets(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedConfig(s, sec, u); err != nil {
		t.Fatal(err)
	}
	for _, app := range ServarrApps {
		key := sec.APIKeys[app]
		if len(key) != 32 {
			t.Fatalf("%s key length %d, want 32 hex chars", app, len(key))
		}
		raw, err := os.ReadFile(filepath.Join(s.ConfigDir(app), "config.xml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "<ApiKey>"+key+"</ApiKey>") {
			t.Fatalf("%s config.xml does not carry the generated key", app)
		}
	}
	if len(sec.JellyfinAdminPassword) < 20 {
		t.Fatal("jellyfin admin password shorter than 20 chars (X-8)")
	}

	// Second run reuses the same secrets and accepts the seeded config.
	sec2, err := LoadOrCreateSecrets(s)
	if err != nil {
		t.Fatal(err)
	}
	if sec2.APIKeys["radarr"] != sec.APIKeys["radarr"] || sec2.JellyfinAdminPassword != sec.JellyfinAdminPassword {
		t.Fatal("secrets regenerated on second run")
	}
	if err := SeedConfig(s, sec2, u); err != nil {
		t.Fatalf("second seed: %v", err)
	}
}

func TestT5_UniqueKeysPerApp(t *testing.T) {
	s := mustParse(t, validYAML, t.TempDir())
	sec, err := LoadOrCreateSecrets(s)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, v := range sec.Values() {
		if seen[v] {
			t.Fatal("secret values must be unique")
		}
		seen[v] = true
	}
}

func TestT5_SecretsFileOwnerOnly(t *testing.T) {
	s := mustParse(t, validYAML, t.TempDir())
	if _, err := LoadOrCreateSecrets(s); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(s.SecretsPath())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file mode %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(s.SecretsPath()))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("secrets dir mode %v, want 0700", di.Mode().Perm())
	}
}

func TestT5_KeyDriftRefusedWithoutLeakingKey(t *testing.T) {
	s := mustParse(t, validYAML, t.TempDir())
	u := User{UID: os.Getuid(), GID: os.Getgid()}
	sec, _ := LoadOrCreateSecrets(s)
	if err := SeedConfig(s, sec, u); err != nil {
		t.Fatal(err)
	}
	other := "ffffffffffffffffffffffffffffffff"
	path := filepath.Join(s.ConfigDir("radarr"), "config.xml")
	if err := os.WriteFile(path, []byte("<Config><ApiKey>"+other+"</ApiKey></Config>"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SeedConfig(s, sec, u)
	var de *KeyDriftError
	if !errors.As(err, &de) {
		t.Fatalf("want KeyDriftError, got %v", err)
	}
	if strings.Contains(err.Error(), other) || strings.Contains(err.Error(), sec.APIKeys["radarr"]) {
		t.Fatal("drift error leaks a key")
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), other) {
		t.Fatal("drifted config.xml was overwritten")
	}
}

func TestD11_ProjectNameCollisionRefused(t *testing.T) {
	existing := []Project{
		{Name: "hli", ConfigFiles: []string{"/home/a/stack-one/docker-compose.yml"}},
		{Name: "other", ConfigFiles: []string{"/x/docker-compose.yml"}},
	}
	err := GuardProject(existing, "hli", "/home/a/stack-two/docker-compose.yml")
	var pe *ProjectCollisionError
	if !errors.As(err, &pe) {
		t.Fatalf("want ProjectCollisionError, got %v", err)
	}
	if err := GuardProject(existing, "hli", "/home/a/stack-one/docker-compose.yml"); err != nil {
		t.Fatalf("same stack dir must be allowed: %v", err)
	}
	if err := GuardProject(existing, "fresh", "/home/a/stack-two/docker-compose.yml"); err != nil {
		t.Fatalf("unused name must be allowed: %v", err)
	}
}

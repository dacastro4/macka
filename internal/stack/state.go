// Package stack loads stack.yml, the single source of truth (T-2), and
// generates the Compose project from it.
package stack

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

// SchemaVersion is the only stack.yml version this build understands.
const SchemaVersion = 1

// CoreApps is the M0 core video stack, in start order.
var CoreApps = []string{"prowlarr", "radarr", "sonarr", "jellyfin"}

// State is the parsed stack.yml.
type State struct {
	Version  int    `yaml:"version"`
	Project  string `yaml:"project"`
	Timezone string `yaml:"timezone"`
	// BindAddress is where app ports are published: loopback or a private LAN IP (X-3).
	BindAddress string   `yaml:"bindAddress"`
	Paths       Paths    `yaml:"paths"`
	User        *User    `yaml:"user,omitempty"`
	Apps        []string `yaml:"apps"`

	// Dir is the directory holding stack.yml; relative paths resolve against it.
	Dir string `yaml:"-"`
}

// Paths are host paths. Relative paths resolve against the stack directory.
type Paths struct {
	Data string `yaml:"data"`
}

// User is the uid/gid containers run as. Defaults to the stack directory owner.
type User struct {
	UID int `yaml:"uid"`
	GID int `yaml:"gid"`
}

// ValidationError names the invalid field.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("stack.yml: %s: %s", e.Field, e.Reason)
}

var projectNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// Load reads and validates stack.yml at path.
func Load(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read stack file: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve stack file path: %w", err)
	}
	return Parse(raw, filepath.Dir(abs))
}

// Parse decodes and validates stack.yml content. Unknown fields are rejected.
func Parse(raw []byte, dir string) (*State, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var s State
	if err := dec.Decode(&s); err != nil {
		return nil, &ValidationError{Field: "(document)", Reason: err.Error()}
	}
	s.Dir = dir
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate checks every field and returns the first problem found.
func (s *State) Validate() error {
	if s.Version != SchemaVersion {
		return &ValidationError{"version", fmt.Sprintf("must be %d", SchemaVersion)}
	}
	if !projectNameRE.MatchString(s.Project) {
		return &ValidationError{"project", "lowercase letters, digits, '-' or '_', starting with a letter or digit"}
	}
	if s.Timezone == "" {
		return &ValidationError{"timezone", "required, e.g. Etc/UTC"}
	}
	if err := validateBind(s.BindAddress); err != nil {
		return err
	}
	if s.Paths.Data == "" {
		return &ValidationError{"paths.data", "required"}
	}
	if s.User != nil && (s.User.UID < 0 || s.User.GID < 0) {
		return &ValidationError{"user", "uid and gid must be non-negative"}
	}
	if len(s.Apps) == 0 {
		return &ValidationError{"apps", "at least one app is required"}
	}
	seen := map[string]bool{}
	for _, a := range s.Apps {
		if !slices.Contains(CoreApps, a) {
			return &ValidationError{"apps", fmt.Sprintf("unknown app %q (M0 supports %v)", a, CoreApps)}
		}
		if seen[a] {
			return &ValidationError{"apps", fmt.Sprintf("duplicate app %q", a)}
		}
		seen[a] = true
	}
	return nil
}

// validateBind enforces X-3: loopback or a private LAN address, never all interfaces.
func validateBind(addr string) error {
	ip := net.ParseIP(addr)
	if ip == nil {
		return &ValidationError{"bindAddress", "must be an IP address"}
	}
	if ip.IsUnspecified() {
		return &ValidationError{"bindAddress", "must not be 0.0.0.0 or :: (apps bind to LAN or localhost only)"}
	}
	if !ip.IsLoopback() && !ip.IsPrivate() {
		return &ValidationError{"bindAddress", "must be loopback or a private LAN address"}
	}
	return nil
}

// Has reports whether app is enabled.
func (s *State) Has(app string) bool {
	return slices.Contains(s.Apps, app)
}

// OrderedApps returns enabled apps in CoreApps order, so output is deterministic.
func (s *State) OrderedApps() []string {
	out := make([]string, 0, len(s.Apps))
	for _, a := range CoreApps {
		if s.Has(a) {
			out = append(out, a)
		}
	}
	return out
}

// Abs resolves p against the stack directory.
func (s *State) Abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(s.Dir, p)
}

// DataRoot is the absolute host path of the shared data root (S-1).
func (s *State) DataRoot() string { return s.Abs(s.Paths.Data) }

// ConfigDir is the absolute host path of an app's /config.
func (s *State) ConfigDir(app string) string { return filepath.Join(s.Dir, "config", app) }

// SecretsPath is the owner-only secrets file.
func (s *State) SecretsPath() string { return filepath.Join(s.Dir, "secrets", "secrets.json") }

// ComposePath and EnvPath are the generated files.
func (s *State) ComposePath() string { return filepath.Join(s.Dir, "docker-compose.yml") }
func (s *State) EnvPath() string     { return filepath.Join(s.Dir, ".env") }

// ErrNoUser is returned when the owner of the stack directory cannot be read.
var ErrNoUser = errors.New("cannot determine uid/gid; set user.uid and user.gid in stack.yml")

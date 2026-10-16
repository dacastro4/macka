package stack

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// KeyDriftError means an app's config.xml holds a key macka did not generate.
// The key value is never included in the message (X-10).
type KeyDriftError struct{ App string }

func (e *KeyDriftError) Error() string {
	return fmt.Sprintf("%s config.xml has an API key that differs from the macka secret; refusing to overwrite (drift)", e.App)
}

type servarrConfig struct {
	XMLName xml.Name `xml:"Config"`
	APIKey  string   `xml:"ApiKey"`
}

// SeedConfig prepares host folders and pre-seeds Servarr API keys before first
// start (T-5). An existing config.xml is left alone if its key matches.
func SeedConfig(s *State, sec Secrets, u User) error {
	for _, app := range s.OrderedApps() {
		if err := mkdirOwned(s.ConfigDir(app), u); err != nil {
			return err
		}
	}
	if s.Has("jellyfin") {
		if err := mkdirOwned(filepath.Join(s.Dir, "cache", "jellyfin"), u); err != nil {
			return err
		}
	}
	for _, kind := range []string{"movies", "tv"} {
		if err := mkdirOwned(filepath.Join(s.DataRoot(), MediaDirs[kind]), u); err != nil {
			return err
		}
	}
	for _, app := range ServarrApps {
		if !s.Has(app) {
			continue
		}
		if err := seedServarrKey(s.ConfigDir(app), app, sec.APIKeys[app], u); err != nil {
			return err
		}
	}
	return nil
}

func seedServarrKey(dir, app, key string, u User) error {
	path := filepath.Join(dir, "config.xml")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		body := "<Config>\n  <ApiKey>" + key + "</ApiKey>\n</Config>\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return fmt.Errorf("seed %s config: %w", app, err)
		}
		return chown(path, u)
	}
	if err != nil {
		return fmt.Errorf("read %s config: %w", app, err)
	}
	var c servarrConfig
	if err := xml.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("%s config.xml is not valid XML", app)
	}
	if c.APIKey != key {
		return &KeyDriftError{App: app}
	}
	return nil
}

func mkdirOwned(dir string, u User) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return chown(dir, u)
}

// chown sets ownership when running as root; as a normal user the files are
// already owned by that user, which ResolveUser defaults to.
func chown(path string, u User) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if err := os.Chown(path, u.UID, u.GID); err != nil {
		return fmt.Errorf("chown %s: %w", path, err)
	}
	return nil
}

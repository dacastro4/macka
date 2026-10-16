package stack

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
)

// ServarrApps take a pre-seeded API key in config.xml (T-5, verified).
var ServarrApps = []string{"prowlarr", "radarr", "sonarr"}

// Secrets are generated once and stored owner-only (X-10).
type Secrets struct {
	APIKeys               map[string]string `json:"apiKeys"`
	JellyfinAdminUser     string            `json:"jellyfinAdminUser"`
	JellyfinAdminPassword string            `json:"jellyfinAdminPassword"`
}

// Values returns every secret value, for log redaction.
func (s Secrets) Values() []string {
	out := []string{s.JellyfinAdminPassword}
	keys := make([]string, 0, len(s.APIKeys))
	for k := range s.APIKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, s.APIKeys[k])
	}
	return out
}

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// PasswordLength meets X-8 (20 or more characters).
const PasswordLength = 24

// NewAPIKey returns 32 lowercase hex characters, the Servarr key format.
func NewAPIKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NewPassword returns a random password of PasswordLength characters.
func NewPassword() (string, error) {
	out := make([]byte, PasswordLength)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// LoadOrCreateSecrets reads the secrets file, generating any value that is
// missing for the enabled apps, and keeps the file and directory owner-only.
func LoadOrCreateSecrets(s *State) (Secrets, error) {
	path := s.SecretsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Secrets{}, fmt.Errorf("create secrets dir: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return Secrets{}, fmt.Errorf("secure secrets dir: %w", err)
	}

	sec := Secrets{APIKeys: map[string]string{}}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &sec); err != nil {
			return Secrets{}, fmt.Errorf("secrets file %s is not valid JSON", path)
		}
		if sec.APIKeys == nil {
			sec.APIKeys = map[string]string{}
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return Secrets{}, fmt.Errorf("read secrets: %w", err)
	}

	changed, err := fillSecrets(s, &sec)
	if err != nil {
		return Secrets{}, err
	}
	if changed {
		out, err := json.MarshalIndent(sec, "", "  ")
		if err != nil {
			return Secrets{}, fmt.Errorf("encode secrets: %w", err)
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
			return Secrets{}, fmt.Errorf("write secrets: %w", err)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return Secrets{}, fmt.Errorf("secure secrets file: %w", err)
	}
	return sec, nil
}

func fillSecrets(s *State, sec *Secrets) (bool, error) {
	changed := false
	for _, app := range ServarrApps {
		if !s.Has(app) || sec.APIKeys[app] != "" {
			continue
		}
		k, err := NewAPIKey()
		if err != nil {
			return false, err
		}
		sec.APIKeys[app] = k
		changed = true
	}
	if s.Has("jellyfin") {
		if sec.JellyfinAdminUser == "" {
			sec.JellyfinAdminUser = "admin"
			changed = true
		}
		if sec.JellyfinAdminPassword == "" {
			p, err := NewPassword()
			if err != nil {
				return false, err
			}
			sec.JellyfinAdminPassword = p
			changed = true
		}
	}
	return changed, nil
}

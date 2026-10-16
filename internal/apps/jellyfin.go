package apps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

// KeyAppName is the AppName of the API key macka creates in Jellyfin.
const KeyAppName = "macka"

// Jellyfin talks to Jellyfin (COMPATIBILITY.md: Jellyfin).
type Jellyfin struct {
	base     string
	deviceID string
	user     string
	password string

	mu  sync.Mutex
	key string
}

// NewJellyfin builds a client that signs in as the macka admin user.
func NewJellyfin(baseURL, deviceID, user, password string) *Jellyfin {
	return &Jellyfin{base: baseURL, deviceID: deviceID, user: user, password: password}
}

func (j *Jellyfin) clientHeader() string {
	return fmt.Sprintf(`MediaBrowser Client="macka", Device="macka", DeviceId=%q, Version="0.1.0"`, j.deviceID)
}

func (j *Jellyfin) anon() *client {
	return newClient(j.base, func(r *http.Request) { r.Header.Set("Authorization", j.clientHeader()) })
}

func (j *Jellyfin) withToken(token string) *client {
	h := fmt.Sprintf(`%s, Token=%q`, j.clientHeader(), token)
	return newClient(j.base, func(r *http.Request) { r.Header.Set("Authorization", h) })
}

type publicInfo struct {
	Version                string `json:"Version"`
	StartupWizardCompleted bool   `json:"StartupWizardCompleted"`
}

// Public returns unauthenticated server info; also the readiness probe.
func (j *Jellyfin) Public(ctx context.Context) (publicInfo, error) {
	var pi publicInfo
	err := j.anon().do(ctx, http.MethodGet, "/System/Info/Public", nil, &pi)
	return pi, err
}

// Ping is the readiness probe.
func (j *Jellyfin) Ping(ctx context.Context) error {
	_, err := j.Public(ctx)
	return err
}

// CompleteStartup runs the first-run wizard over the API: server settings,
// admin user, remote access off (R-1), complete.
func (j *Jellyfin) CompleteStartup(ctx context.Context, serverName string) error {
	c := j.anon()
	var first map[string]any
	if err := c.do(ctx, http.MethodGet, "/Startup/User", nil, &first); err != nil {
		return err
	}
	steps := []struct {
		path string
		body any
	}{
		{"/Startup/Configuration", map[string]string{
			"ServerName": serverName, "UICulture": "en-US",
			"MetadataCountryCode": "US", "PreferredMetadataLanguage": "en",
		}},
		{"/Startup/User", map[string]string{"Name": j.user, "Password": j.password}},
		{"/Startup/RemoteAccess", map[string]bool{"EnableRemoteAccess": false}},
		{"/Startup/Complete", nil},
	}
	for _, s := range steps {
		if err := c.do(ctx, http.MethodPost, s.path, s.body, nil); err != nil {
			return err
		}
	}
	return nil
}

// ErrBadLogin means the admin credentials were rejected.
var ErrBadLogin = errors.New("jellyfin rejected the macka admin credentials")

// Login returns a session token for the admin user.
func (j *Jellyfin) Login(ctx context.Context) (string, error) {
	var out struct {
		AccessToken string `json:"AccessToken"`
	}
	err := j.anon().do(ctx, http.MethodPost, "/Users/AuthenticateByName",
		map[string]string{"Username": j.user, "Pw": j.password}, &out)
	if isStatus(err, http.StatusUnauthorized) {
		return "", ErrBadLogin
	}
	if err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("jellyfin login returned no token")
	}
	return out.AccessToken, nil
}

// FindKey returns the API key with AppName KeyAppName, or "".
func (j *Jellyfin) FindKey(ctx context.Context) (string, error) {
	tok, err := j.Login(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Items []struct {
			AccessToken string `json:"AccessToken"`
			AppName     string `json:"AppName"`
		} `json:"Items"`
	}
	if err := j.withToken(tok).do(ctx, http.MethodGet, "/Auth/Keys", nil, &out); err != nil {
		return "", err
	}
	for _, k := range out.Items {
		if k.AppName == KeyAppName {
			return k.AccessToken, nil
		}
	}
	return "", nil
}

// CreateKey creates the macka API key.
func (j *Jellyfin) CreateKey(ctx context.Context) error {
	tok, err := j.Login(ctx)
	if err != nil {
		return err
	}
	return j.withToken(tok).do(ctx, http.MethodPost, "/Auth/Keys?app="+KeyAppName, nil, nil)
}

// Key returns the macka API key, cached after the first successful lookup.
func (j *Jellyfin) Key(ctx context.Context) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.key != "" {
		return j.key, nil
	}
	k, err := j.FindKey(ctx)
	if err != nil {
		return "", err
	}
	if k == "" {
		return "", errors.New("jellyfin has no macka API key yet")
	}
	j.key = k
	return k, nil
}

func (j *Jellyfin) keyed(ctx context.Context) (*client, error) {
	k, err := j.Key(ctx)
	if err != nil {
		return nil, err
	}
	h := fmt.Sprintf(`MediaBrowser Token=%q`, k)
	return newClient(j.base, func(r *http.Request) { r.Header.Set("Authorization", h) }), nil
}

// VirtualFolder is a Jellyfin library.
type VirtualFolder struct {
	Name           string   `json:"Name"`
	CollectionType string   `json:"CollectionType"`
	Locations      []string `json:"Locations"`
}

// Libraries lists libraries using the macka API key.
func (j *Jellyfin) Libraries(ctx context.Context) ([]VirtualFolder, error) {
	c, err := j.keyed(ctx)
	if err != nil {
		return nil, err
	}
	var out []VirtualFolder
	err = c.do(ctx, http.MethodGet, "/Library/VirtualFolders", nil, &out)
	return out, err
}

// AddLibrary creates a library without starting a scan.
func (j *Jellyfin) AddLibrary(ctx context.Context, name, collectionType, path string) error {
	c, err := j.keyed(ctx)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("collectionType", collectionType)
	q.Set("paths", path)
	q.Set("refreshLibrary", "false")
	return c.do(ctx, http.MethodPost, "/Library/VirtualFolders?"+q.Encode(),
		map[string]any{"LibraryOptions": map[string]any{}}, nil)
}

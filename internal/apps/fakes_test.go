package apps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeServarr mimics the Servarr API shapes recorded in docs/COMPATIBILITY.md.
type fakeServarr struct {
	mu      sync.Mutex
	key     string
	data    map[string][]Record
	schema  map[string][]Record
	nextID  int
	creates int
	// validKeys maps a target base URL to its key, for Prowlarr's create-time test.
	validKeys map[string]string
}

func newFakeServarr(key string) *fakeServarr {
	return &fakeServarr{key: key, data: map[string][]Record{}, schema: map[string][]Record{}, nextID: 1, validKeys: map[string]string{}}
}

func field(name string, value any) map[string]any {
	return map[string]any{"name": name, "value": value}
}

func prowlarrSchema() []Record {
	return []Record{
		{"implementation": "Radarr", "syncLevel": "fullSync", "enable": true, "tags": []any{},
			"fields": []any{field("prowlarrUrl", "http://localhost:9696"), field("baseUrl", "http://localhost:7878"), field("apiKey", nil), field("syncCategories", []any{2000})}},
		{"implementation": "Sonarr", "syncLevel": "fullSync", "enable": true, "tags": []any{},
			"fields": []any{field("prowlarrUrl", "http://localhost:9696"), field("baseUrl", "http://localhost:8989"), field("apiKey", nil), field("syncCategories", []any{5000})}},
	}
}

func notificationSchema(deleteFlag string) []Record {
	return []Record{{
		"implementation": "MediaBrowser", "implementationName": "Emby / Jellyfin", "tags": []any{},
		"onGrab": false, "onDownload": false, "onUpgrade": false, "onRename": false,
		deleteFlag: false, "supportsO" + deleteFlag[1:]: true,
		"onHealthIssue": false, "supportsOnHealthIssue": true,
		"fields": []any{field("host", nil), field("port", 8096), field("useSsl", false), field("apiKey", nil), field("updateLibrary", true)},
	}}
}

func masked(r Record) Record {
	b, _ := json.Marshal(r)
	var out Record
	_ = json.Unmarshal(b, &out)
	if fs, ok := out["fields"].([]any); ok {
		for _, f := range fs {
			fm := f.(map[string]any)
			if fm["name"] == "apiKey" && fm["value"] != nil {
				fm["value"] = "********"
			}
		}
	}
	return out
}

var servarrPath = regexp.MustCompile(`^/api/v[13]/([a-z]+)(?:/(schema|test|\d+))?$`)

func (f *fakeServarr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ping" {
		w.WriteHeader(200)
		return
	}
	if r.Header.Get("X-Api-Key") != f.key {
		w.WriteHeader(401)
		return
	}
	m := servarrPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		w.WriteHeader(404)
		return
	}
	res, sub := m[1], m[2]
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == "GET" && sub == "schema":
		writeJSON(w, 200, f.schema[res])
	case r.Method == "GET" && sub == "":
		out := []Record{}
		for _, rec := range f.data[res] {
			out = append(out, masked(rec))
		}
		writeJSON(w, 200, out)
	case r.Method == "GET":
		id, _ := strconv.Atoi(sub)
		for _, rec := range f.data[res] {
			if rec["id"] == float64(id) {
				writeJSON(w, 200, masked(rec))
				return
			}
		}
		w.WriteHeader(404)
	case r.Method == "POST" && sub == "test":
		var rec Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		if rec["id"] == nil && FindByName(f.data[res], rec["name"].(string)) != nil {
			writeJSON(w, 400, []any{map[string]string{"errorMessage": "Should be unique"}})
			return
		}
		writeJSON(w, 200, map[string]any{})
	case r.Method == "POST" && sub == "":
		var rec Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		if name, ok := rec["name"].(string); ok && FindByName(f.data[res], name) != nil {
			writeJSON(w, 400, []any{map[string]string{"errorMessage": "Should be unique"}})
			return
		}
		if res == "applications" {
			base, _ := FieldValue(rec, "baseUrl")
			key, _ := FieldValue(rec, "apiKey")
			if want, ok := f.validKeys[fmt.Sprint(base)]; ok && want != key {
				writeJSON(w, 400, []any{map[string]string{"errorMessage": "API Key is invalid"}})
				return
			}
		}
		if res == "rootfolder" {
			rec["accessible"] = true
		}
		rec["id"] = float64(f.nextID)
		f.nextID++
		f.creates++
		f.data[res] = append(f.data[res], rec)
		writeJSON(w, 201, masked(rec))
	default:
		w.WriteHeader(405)
	}
}

// fakeJellyfin mimics the Jellyfin startup, auth, key and library API.
type fakeJellyfin struct {
	mu        sync.Mutex
	completed bool
	user, pw  string
	sessions  map[string]bool
	keys      map[string]string // token -> app name
	libs      []VirtualFolder
	mutations int
}

func newFakeJellyfin() *fakeJellyfin {
	return &fakeJellyfin{sessions: map[string]bool{}, keys: map[string]string{}}
}

var tokenRE = regexp.MustCompile(`Token="([^"]+)"`)

func (f *fakeJellyfin) token(r *http.Request) string {
	m := tokenRE.FindStringSubmatch(r.Header.Get("Authorization"))
	if m == nil {
		return ""
	}
	return m[1]
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := f.token(r)
	elevated := f.sessions[tok] || f.keys[tok] != ""
	firstRun := !f.completed
	p := r.URL.Path
	switch {
	case p == "/System/Info/Public":
		writeJSON(w, 200, map[string]any{"Version": "12.2.0", "StartupWizardCompleted": f.completed})
	case strings.HasPrefix(p, "/Startup/"):
		if !firstRun && !elevated {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET" && p == "/Startup/User":
			writeJSON(w, 200, map[string]string{"Name": "MyJellyfinUser"})
		case p == "/Startup/User":
			var b struct{ Name, Password string }
			_ = json.NewDecoder(r.Body).Decode(&b)
			f.user, f.pw = b.Name, b.Password
			f.mutations++
			w.WriteHeader(204)
		case p == "/Startup/Complete":
			f.completed = true
			f.mutations++
			w.WriteHeader(204)
		default:
			w.WriteHeader(204)
		}
	case p == "/Users/AuthenticateByName":
		var b struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if !f.completed || b.Username != f.user || b.Pw != f.pw || b.Pw == "" {
			w.WriteHeader(401)
			return
		}
		t := fmt.Sprintf("session%026d", len(f.sessions))
		f.sessions[t] = true
		writeJSON(w, 200, map[string]string{"AccessToken": t})
	case p == "/Auth/Keys":
		if !f.sessions[tok] {
			w.WriteHeader(401)
			return
		}
		if r.Method == "POST" {
			k := fmt.Sprintf("apikey%026d", len(f.keys))
			f.keys[k] = r.URL.Query().Get("app")
			f.mutations++
			w.WriteHeader(204)
			return
		}
		items := []map[string]string{}
		for k, app := range f.keys {
			items = append(items, map[string]string{"AccessToken": k, "AppName": app})
		}
		writeJSON(w, 200, map[string]any{"Items": items})
	case p == "/Library/VirtualFolders":
		if !elevated {
			w.WriteHeader(401)
			return
		}
		if r.Method == "POST" {
			q := r.URL.Query()
			f.libs = append(f.libs, VirtualFolder{Name: q.Get("name"), CollectionType: q.Get("collectionType"), Locations: []string{q.Get("paths")}})
			f.mutations++
			w.WriteHeader(204)
			return
		}
		writeJSON(w, 200, f.libs)
	default:
		w.WriteHeader(404)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func portOf(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	i := strings.LastIndex(srv.URL, ":")
	p, err := strconv.Atoi(srv.URL[i+1:])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Record is a Servarr resource as returned by its API.
type Record = map[string]any

// Servarr talks to Radarr, Sonarr or Prowlarr (COMPATIBILITY.md: Servarr).
type Servarr struct {
	Name string // display name, e.g. "Radarr"
	c    *client
	api  string // e.g. "/api/v3"
}

// NewServarr builds a client. apiPath is "/api/v3" (Radarr, Sonarr) or "/api/v1" (Prowlarr).
func NewServarr(name, baseURL, apiPath, key string) *Servarr {
	return &Servarr{
		Name: name,
		api:  apiPath,
		c:    newClient(baseURL, func(r *http.Request) { r.Header.Set("X-Api-Key", key) }),
	}
}

// Ping returns nil once the app answers GET /ping.
func (s *Servarr) Ping(ctx context.Context) error {
	return s.c.do(ctx, http.MethodGet, "/ping", nil, nil)
}

// List returns all records of resource, e.g. "rootfolder".
func (s *Servarr) List(ctx context.Context, resource string) ([]Record, error) {
	var out []Record
	err := s.c.do(ctx, http.MethodGet, s.api+"/"+resource, nil, &out)
	return out, err
}

// Get returns one record by id.
func (s *Servarr) Get(ctx context.Context, resource string, id any) (Record, error) {
	var out Record
	err := s.c.do(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%v", s.api, resource, id), nil, &out)
	return out, err
}

// Schema returns the templates the app offers for resource (T-6).
func (s *Servarr) Schema(ctx context.Context, resource string) ([]Record, error) {
	return s.List(ctx, resource+"/schema")
}

// Create posts a new record. The app runs its own connection test first.
func (s *Servarr) Create(ctx context.Context, resource string, rec any) (Record, error) {
	var out Record
	err := s.c.do(ctx, http.MethodPost, s.api+"/"+resource, rec, &out)
	return out, err
}

// Test asks the app to test an existing record (masked secrets are accepted).
func (s *Servarr) Test(ctx context.Context, resource string, rec Record) error {
	return s.c.do(ctx, http.MethodPost, s.api+"/"+resource+"/test", rec, nil)
}

// FindByName returns the record whose "name" equals name, or nil.
func FindByName(recs []Record, name string) Record {
	for _, r := range recs {
		if r["name"] == name {
			return r
		}
	}
	return nil
}

// FieldValue returns the value of a named entry in rec["fields"].
func FieldValue(rec Record, name string) (any, bool) {
	fields, _ := rec["fields"].([]any)
	for _, f := range fields {
		fm, _ := f.(map[string]any)
		if fm["name"] == name {
			v, ok := fm["value"]
			return v, ok
		}
	}
	return nil, false
}

// FromSchema builds a create payload from the app's own template (T-6): it
// copies the template for implementation, sets name, fills named fields and
// top-level flags. A field or flag the template lacks is an error, so macka
// never invents payload keys.
func FromSchema(schema []Record, implementation, name string, fields, flags map[string]any) (Record, error) {
	var tmpl Record
	for _, r := range schema {
		if r["implementation"] == implementation {
			tmpl = r
			break
		}
	}
	if tmpl == nil {
		return nil, fmt.Errorf("schema has no %q template", implementation)
	}
	rec, err := deepCopy(tmpl)
	if err != nil {
		return nil, err
	}
	rec["name"] = name
	for k, v := range flags {
		if _, ok := rec[k]; !ok {
			return nil, fmt.Errorf("%s template has no %q property", implementation, k)
		}
		rec[k] = v
	}
	list, _ := rec["fields"].([]any)
	for k, v := range fields {
		found := false
		for _, f := range list {
			fm, _ := f.(map[string]any)
			if fm["name"] == k {
				fm["value"] = v
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%s template has no %q field", implementation, k)
		}
	}
	return rec, nil
}

// SupportedDeleteFlags returns every on*Delete* flag the template supports
// (Radarr and Sonarr name them differently; COMPATIBILITY.md).
func SupportedDeleteFlags(tmpl Record) []string {
	var out []string
	for k := range tmpl {
		if strings.HasPrefix(k, "on") && strings.Contains(k, "Delete") && tmpl["supportsO"+k[1:]] == true {
			out = append(out, k)
		}
	}
	return out
}

func deepCopy(r Record) (Record, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var out Record
	err = json.Unmarshal(b, &out)
	return out, err
}

// fieldMismatches lists fields whose current value differs from want.
func fieldMismatches(rec Record, want map[string]any) []string {
	var out []string
	for k, v := range want {
		got, _ := FieldValue(rec, k)
		if fmt.Sprint(got) != fmt.Sprint(v) {
			out = append(out, fmt.Sprintf("%s is %v, want %v", k, got, v))
		}
	}
	return out
}

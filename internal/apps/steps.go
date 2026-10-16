package apps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dacastro4/macka/internal/wiring"
)

// base holds the identity fields every step shares.
type base struct {
	id, label string
	targets   []string
	deps      []string
}

func (b base) ID() string          { return b.id }
func (b base) Label() string       { return b.label }
func (b base) Targets() []string   { return b.targets }
func (b base) DependsOn() []string { return b.deps }

var (
	satisfied = wiring.CheckResult{State: wiring.Satisfied}
	missing   = wiring.CheckResult{State: wiring.Missing}
)

func drifted(details ...string) wiring.CheckResult {
	return wiring.CheckResult{State: wiring.Drifted, Detail: strings.Join(details, "; ")}
}

// RootFolderStep makes path a root folder in a Radarr or Sonarr instance.
type RootFolderStep struct {
	base
	App  *Servarr
	Path string
}

// NewRootFolderStep builds the step for app (target name) and path.
func NewRootFolderStep(target string, app *Servarr, path string) *RootFolderStep {
	return &RootFolderStep{
		base: base{id: target + ":rootfolder:" + path, label: app.Name + " media folder " + path, targets: []string{target}},
		App:  app, Path: path,
	}
}

func (s *RootFolderStep) find(ctx context.Context) (Record, error) {
	recs, err := s.App.List(ctx, "rootfolder")
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if r["path"] == s.Path {
			return r, nil
		}
	}
	return nil, nil
}

func (s *RootFolderStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	r, err := s.find(ctx)
	if err != nil || r == nil {
		return missing, err
	}
	return satisfied, nil
}

func (s *RootFolderStep) Apply(ctx context.Context) error {
	_, err := s.App.Create(ctx, "rootfolder", map[string]string{"path": s.Path})
	return err
}

func (s *RootFolderStep) Verify(ctx context.Context) error {
	r, err := s.find(ctx)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("%s does not list %s as a root folder", s.App.Name, s.Path)
	}
	if r["accessible"] != true {
		return fmt.Errorf("%s reports %s as not accessible", s.App.Name, s.Path)
	}
	return nil
}

// ProwlarrAppStep registers a Radarr or Sonarr instance in Prowlarr with full sync.
type ProwlarrAppStep struct {
	base
	Prowlarr       *Servarr
	Implementation string // "Radarr" or "Sonarr"
	Fields         map[string]any
	apiKey         string
}

// NewProwlarrAppStep builds the link Prowlarr → target.
func NewProwlarrAppStep(prowlarr *Servarr, target, implementation, targetURL, prowlarrURL, targetKey string) *ProwlarrAppStep {
	return &ProwlarrAppStep{
		base: base{
			id:      "prowlarr->" + target + ":app",
			label:   "Prowlarr to " + implementation,
			targets: []string{"prowlarr", target},
		},
		Prowlarr:       prowlarr,
		Implementation: implementation,
		Fields:         map[string]any{"baseUrl": targetURL, "prowlarrUrl": prowlarrURL},
		apiKey:         targetKey,
	}
}

func (s *ProwlarrAppStep) find(ctx context.Context) (Record, error) {
	recs, err := s.Prowlarr.List(ctx, "applications")
	if err != nil {
		return nil, err
	}
	return FindByName(recs, s.Implementation), nil
}

// Check compares everything except the key, which Prowlarr masks; Verify's
// connection test covers the key.
func (s *ProwlarrAppStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	r, err := s.find(ctx)
	if err != nil || r == nil {
		return missing, err
	}
	diffs := fieldMismatches(r, s.Fields)
	if r["implementation"] != s.Implementation {
		diffs = append(diffs, fmt.Sprintf("implementation is %v", r["implementation"]))
	}
	if r["syncLevel"] != "fullSync" {
		diffs = append(diffs, fmt.Sprintf("syncLevel is %v, want fullSync", r["syncLevel"]))
	}
	if len(diffs) > 0 {
		return drifted(diffs...), nil
	}
	return satisfied, nil
}

func (s *ProwlarrAppStep) Apply(ctx context.Context) error {
	schema, err := s.Prowlarr.Schema(ctx, "applications")
	if err != nil {
		return err
	}
	fields := map[string]any{"apiKey": s.apiKey}
	for k, v := range s.Fields {
		fields[k] = v
	}
	rec, err := FromSchema(schema, s.Implementation, s.Implementation, fields, map[string]any{"syncLevel": "fullSync"})
	if err != nil {
		return err
	}
	_, err = s.Prowlarr.Create(ctx, "applications", rec)
	return err
}

func (s *ProwlarrAppStep) Verify(ctx context.Context) error {
	return verifyByTest(ctx, s.Prowlarr, "applications", s.Implementation)
}

// NotificationStep makes Radarr or Sonarr refresh Jellyfin's library on import.
type NotificationStep struct {
	base
	App      *Servarr
	Jellyfin *Jellyfin
	Fields   map[string]any
}

// NotificationName is the name macka gives the Jellyfin connection in each app.
const NotificationName = "Jellyfin"

// NewNotificationStep builds the link app → Jellyfin. It depends on the
// Jellyfin API key step.
func NewNotificationStep(target string, app *Servarr, jf *Jellyfin, host string, port int, keyStepID string) *NotificationStep {
	return &NotificationStep{
		base: base{
			id:      target + "->jellyfin:notification",
			label:   app.Name + " to Jellyfin (library refresh)",
			targets: []string{target, "jellyfin"},
			deps:    []string{keyStepID},
		},
		App:      app,
		Jellyfin: jf,
		Fields:   map[string]any{"host": host, "port": port, "updateLibrary": true},
	}
}

func (s *NotificationStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	recs, err := s.App.List(ctx, "notification")
	if err != nil {
		return missing, err
	}
	r := FindByName(recs, NotificationName)
	if r == nil {
		return missing, nil
	}
	diffs := fieldMismatches(r, s.Fields)
	if r["implementation"] != "MediaBrowser" {
		diffs = append(diffs, fmt.Sprintf("implementation is %v", r["implementation"]))
	}
	if r["onDownload"] != true {
		diffs = append(diffs, "onDownload is off")
	}
	if len(diffs) > 0 {
		return drifted(diffs...), nil
	}
	return satisfied, nil
}

func (s *NotificationStep) Apply(ctx context.Context) error {
	key, err := s.Jellyfin.Key(ctx)
	if err != nil {
		return err
	}
	schema, err := s.App.Schema(ctx, "notification")
	if err != nil {
		return err
	}
	var tmpl Record
	for _, r := range schema {
		if r["implementation"] == "MediaBrowser" {
			tmpl = r
		}
	}
	if tmpl == nil {
		return errors.New("notification schema has no MediaBrowser (Emby / Jellyfin) template")
	}
	flags := map[string]any{"onDownload": true, "onUpgrade": true, "onRename": true}
	for _, f := range SupportedDeleteFlags(tmpl) {
		flags[f] = true
	}
	fields := map[string]any{"apiKey": key}
	for k, v := range s.Fields {
		fields[k] = v
	}
	rec, err := FromSchema(schema, "MediaBrowser", NotificationName, fields, flags)
	if err != nil {
		return err
	}
	_, err = s.App.Create(ctx, "notification", rec)
	return err
}

func (s *NotificationStep) Verify(ctx context.Context) error {
	return verifyByTest(ctx, s.App, "notification", NotificationName)
}

// verifyByTest reads the record back and asks the app to test it
// (COMPATIBILITY.md: "Verify an existing link").
func verifyByTest(ctx context.Context, app *Servarr, resource, name string) error {
	recs, err := app.List(ctx, resource)
	if err != nil {
		return err
	}
	r := FindByName(recs, name)
	if r == nil {
		return fmt.Errorf("%s has no %s named %q", app.Name, resource, name)
	}
	full, err := app.Get(ctx, resource, r["id"])
	if err != nil {
		return err
	}
	if err := app.Test(ctx, resource, full); err != nil {
		return fmt.Errorf("%s connection test failed: %w", app.Name, err)
	}
	return nil
}

// JellyfinSetupStep completes Jellyfin's first-run with the macka admin account.
type JellyfinSetupStep struct {
	base
	J          *Jellyfin
	ServerName string
}

// JellyfinSetupID and JellyfinKeyID are stable step IDs other steps depend on.
const (
	JellyfinSetupID = "jellyfin:setup"
	JellyfinKeyID   = "jellyfin:apikey"
)

func NewJellyfinSetupStep(j *Jellyfin, serverName string) *JellyfinSetupStep {
	return &JellyfinSetupStep{
		base: base{id: JellyfinSetupID, label: "Jellyfin admin account", targets: []string{"jellyfin"}},
		J:    j, ServerName: serverName,
	}
}

func (s *JellyfinSetupStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	pi, err := s.J.Public(ctx)
	if err != nil {
		return missing, err
	}
	if !pi.StartupWizardCompleted {
		return missing, nil
	}
	if _, err := s.J.Login(ctx); errors.Is(err, ErrBadLogin) {
		return drifted("setup was completed outside macka; the macka admin password does not sign in"), nil
	} else if err != nil {
		return missing, err
	}
	return satisfied, nil
}

func (s *JellyfinSetupStep) Apply(ctx context.Context) error {
	return s.J.CompleteStartup(ctx, s.ServerName)
}

func (s *JellyfinSetupStep) Verify(ctx context.Context) error {
	pi, err := s.J.Public(ctx)
	if err != nil {
		return err
	}
	if !pi.StartupWizardCompleted {
		return errors.New("jellyfin still reports first-run setup as incomplete")
	}
	_, err = s.J.Login(ctx)
	return err
}

// JellyfinKeyStep creates the API key macka uses for Jellyfin.
type JellyfinKeyStep struct {
	base
	J *Jellyfin
}

func NewJellyfinKeyStep(j *Jellyfin) *JellyfinKeyStep {
	return &JellyfinKeyStep{
		base: base{id: JellyfinKeyID, label: "Jellyfin API key for macka", targets: []string{"jellyfin"}, deps: []string{JellyfinSetupID}},
		J:    j,
	}
}

func (s *JellyfinKeyStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	k, err := s.J.FindKey(ctx)
	if err != nil || k == "" {
		return missing, err
	}
	return satisfied, nil
}

func (s *JellyfinKeyStep) Apply(ctx context.Context) error { return s.J.CreateKey(ctx) }

func (s *JellyfinKeyStep) Verify(ctx context.Context) error {
	_, err := s.J.Libraries(ctx) // proves the key exists and authenticates
	return err
}

// JellyfinLibraryStep creates a library pointing at a media folder.
type JellyfinLibraryStep struct {
	base
	J              *Jellyfin
	Name           string
	CollectionType string
	Path           string
}

func NewJellyfinLibraryStep(j *Jellyfin, name, collectionType, path string) *JellyfinLibraryStep {
	return &JellyfinLibraryStep{
		base: base{
			id:      "jellyfin:library:" + name,
			label:   "Jellyfin " + name + " library",
			targets: []string{"jellyfin"},
			deps:    []string{JellyfinKeyID},
		},
		J: j, Name: name, CollectionType: collectionType, Path: path,
	}
}

func (s *JellyfinLibraryStep) find(ctx context.Context) (*VirtualFolder, error) {
	libs, err := s.J.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	for i := range libs {
		if libs[i].Name == s.Name {
			return &libs[i], nil
		}
	}
	return nil, nil
}

func (s *JellyfinLibraryStep) Check(ctx context.Context) (wiring.CheckResult, error) {
	lib, err := s.find(ctx)
	if err != nil || lib == nil {
		return missing, err
	}
	var diffs []string
	if lib.CollectionType != s.CollectionType {
		diffs = append(diffs, fmt.Sprintf("type is %q, want %q", lib.CollectionType, s.CollectionType))
	}
	if !slices.Contains(lib.Locations, s.Path) {
		diffs = append(diffs, fmt.Sprintf("folders are %v, want %s", lib.Locations, s.Path))
	}
	if len(diffs) > 0 {
		return drifted(diffs...), nil
	}
	return satisfied, nil
}

func (s *JellyfinLibraryStep) Apply(ctx context.Context) error {
	return s.J.AddLibrary(ctx, s.Name, s.CollectionType, s.Path)
}

func (s *JellyfinLibraryStep) Verify(ctx context.Context) error {
	lib, err := s.find(ctx)
	if err != nil {
		return err
	}
	if lib == nil || lib.CollectionType != s.CollectionType || !slices.Contains(lib.Locations, s.Path) {
		return fmt.Errorf("jellyfin does not list library %q at %s", s.Name, s.Path)
	}
	return nil
}

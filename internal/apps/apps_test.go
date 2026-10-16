package apps

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"hli/internal/compat"
	"hli/internal/stack"
	"hli/internal/wiring"
)

func TestT6_PayloadFromSchema(t *testing.T) {
	schema := prowlarrSchema()
	rec, err := FromSchema(schema, "Radarr", "Radarr",
		map[string]any{"baseUrl": "http://radarr:7878", "apiKey": "k"},
		map[string]any{"syncLevel": "fullSync"})
	if err != nil {
		t.Fatal(err)
	}
	if rec["name"] != "Radarr" || rec["syncLevel"] != "fullSync" {
		t.Fatalf("top-level: %v", rec)
	}
	if v, _ := FieldValue(rec, "baseUrl"); v != "http://radarr:7878" {
		t.Fatalf("baseUrl %v", v)
	}
	if v, _ := FieldValue(rec, "syncCategories"); v == nil {
		t.Fatal("template defaults must be kept")
	}
	if v, _ := FieldValue(schema[0], "baseUrl"); v != "http://localhost:7878" {
		t.Fatal("template was mutated")
	}

	if _, err := FromSchema(schema, "Radarr", "x", map[string]any{"invented": 1}, nil); err == nil {
		t.Fatal("unknown field must be refused")
	}
	if _, err := FromSchema(schema, "Radarr", "x", nil, map[string]any{"inventedFlag": true}); err == nil {
		t.Fatal("unknown flag must be refused")
	}
	if _, err := FromSchema(schema, "Lidarr", "x", nil, nil); err == nil {
		t.Fatal("missing template must be refused")
	}
}

func TestT6_SupportedDeleteFlags(t *testing.T) {
	got := SupportedDeleteFlags(notificationSchema("onSeriesDelete")[0])
	if len(got) != 1 || got[0] != "onSeriesDelete" {
		t.Fatalf("flags %v", got)
	}
}

type rig struct {
	state                    *stack.State
	sec                      stack.Secrets
	pins                     compat.Pins
	prowlarr, radarr, sonarr *fakeServarr
	jf                       *fakeJellyfin
}

func newRig(t *testing.T) *rig {
	t.Helper()
	sec := stack.Secrets{
		APIKeys:               map[string]string{"prowlarr": "pk000000000000000000000000000000", "radarr": "rk000000000000000000000000000000", "sonarr": "sk000000000000000000000000000000"},
		JellyfinAdminUser:     "admin",
		JellyfinAdminPassword: "Pw-For-Tests-123456789x",
	}
	r := &rig{
		sec:      sec,
		prowlarr: newFakeServarr(sec.APIKeys["prowlarr"]),
		radarr:   newFakeServarr(sec.APIKeys["radarr"]),
		sonarr:   newFakeServarr(sec.APIKeys["sonarr"]),
		jf:       newFakeJellyfin(),
	}
	r.prowlarr.schema["applications"] = prowlarrSchema()
	r.radarr.schema["notification"] = notificationSchema("onMovieDelete")
	r.sonarr.schema["notification"] = notificationSchema("onSeriesDelete")

	pins := compat.Pins{}
	servers := map[string]*httptest.Server{
		"prowlarr": httptest.NewServer(r.prowlarr),
		"radarr":   httptest.NewServer(r.radarr),
		"sonarr":   httptest.NewServer(r.sonarr),
		"jellyfin": httptest.NewServer(r.jf),
	}
	for name, srv := range servers {
		t.Cleanup(srv.Close)
		c := compat.Default[name]
		c.Port = portOf(t, srv)
		pins[name] = c
	}
	// Service URLs inside the Compose network use the real container ports.
	r.prowlarr.validKeys = map[string]string{
		"http://radarr:" + strconv.Itoa(pins["radarr"].Port): sec.APIKeys["radarr"],
		"http://sonarr:" + strconv.Itoa(pins["sonarr"].Port): sec.APIKeys["sonarr"],
	}
	r.pins = pins
	st, err := stack.Parse([]byte(`version: 1
project: hli
timezone: Etc/UTC
bindAddress: 127.0.0.1
paths: {data: ./data}
apps: [prowlarr, radarr, sonarr, jellyfin]
`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.state = st
	return r
}

func (r *rig) run(t *testing.T) wiring.Report {
	t.Helper()
	plan, err := Build(r.state, r.sec, r.pins)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := wiring.Engine{Ready: plan.Ready}.Run(context.Background(), plan.Steps)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestCoreVideoStackWiresAllLinks(t *testing.T) {
	r := newRig(t)
	rep := r.run(t)
	if !rep.OK() {
		t.Fatalf("first run not OK:\n%s", strings.Join(rep.Lines(), "\n"))
	}
	want := []string{
		"Radarr media folder /data/media/movies: connected",
		"Sonarr media folder /data/media/tv: connected",
		"Prowlarr to Radarr: connected",
		"Prowlarr to Sonarr: connected",
		"Jellyfin admin account: connected",
		"Jellyfin API key for HLI: connected",
		"Jellyfin Movies library: connected",
		"Jellyfin Shows library: connected",
		"Radarr to Jellyfin (library refresh): connected",
		"Sonarr to Jellyfin (library refresh): connected",
	}
	if got := strings.Join(rep.Lines(), "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("report:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	if rep.Count(wiring.Applied) != len(want) {
		t.Fatalf("first run should apply every link: %+v", rep.Results)
	}

	n := r.radarr.data["notification"][0]
	if n["onDownload"] != true || n["onMovieDelete"] != true || n["onHealthIssue"] == true {
		t.Fatalf("radarr notification flags wrong: %v", n)
	}
	if v, _ := FieldValue(n, "host"); v != "jellyfin" {
		t.Fatalf("notification host %v", v)
	}
	if v, _ := FieldValue(r.sonarr.data["notification"][0], "apiKey"); !strings.HasPrefix(v.(string), "apikey") {
		t.Fatal("notification must carry the jellyfin API key")
	}
	if !r.jf.completed || r.jf.pw != r.sec.JellyfinAdminPassword {
		t.Fatal("jellyfin first-run not completed with the generated password")
	}
}

func TestP4_CoreVideoStackSecondRunChangesNothing(t *testing.T) {
	r := newRig(t)
	r.run(t)
	creates := r.prowlarr.creates + r.radarr.creates + r.sonarr.creates
	mut := r.jf.mutations

	rep := r.run(t)
	if rep.Count(wiring.AlreadySatisfied) != len(rep.Results) {
		t.Fatalf("second run must be all already-satisfied:\n%+v", rep.Results)
	}
	if got := r.prowlarr.creates + r.radarr.creates + r.sonarr.creates; got != creates {
		t.Fatalf("second run created %d servarr records", got-creates)
	}
	if r.jf.mutations != mut {
		t.Fatal("second run changed jellyfin")
	}
}

func TestProwlarrRejectsWrongKey(t *testing.T) {
	r := newRig(t)
	r.sec.APIKeys["radarr"] = "wrongwrongwrongwrongwrongwrong00"
	r.radarr.key = r.sec.APIKeys["radarr"] // the radarr API itself accepts it
	rep := r.run(t)
	for _, res := range rep.Results {
		if res.Label == "Prowlarr to Radarr" {
			if res.Outcome != wiring.Failed || !strings.Contains(res.Detail, "API Key is invalid") {
				t.Fatalf("want failed with key error, got %+v", res)
			}
			return
		}
	}
	t.Fatal("no Prowlarr to Radarr result")
}

func TestJellyfinSetupByOthersIsDrift(t *testing.T) {
	r := newRig(t)
	r.jf.completed, r.jf.user, r.jf.pw = true, "someone", "their-own-password"
	rep := r.run(t)
	for _, res := range rep.Results {
		switch {
		case res.Label == "Jellyfin admin account":
			if res.Outcome != wiring.DriftFound {
				t.Fatalf("want drift, got %+v", res)
			}
		case strings.HasPrefix(res.Label, "Jellyfin"), strings.Contains(res.Label, "to Jellyfin"):
			if res.Outcome != wiring.Blocked {
				t.Fatalf("%s should be blocked, got %+v", res.Label, res)
			}
		}
	}
	if r.jf.pw != "their-own-password" {
		t.Fatal("existing jellyfin admin was overwritten")
	}
}

func TestProwlarrAppDriftDetected(t *testing.T) {
	r := newRig(t)
	r.run(t)
	app := r.prowlarr.data["applications"][0]
	for _, f := range app["fields"].([]any) {
		fm := f.(map[string]any)
		if fm["name"] == "baseUrl" {
			fm["value"] = "http://elsewhere:7878"
		}
	}
	rep := r.run(t)
	res := rep.Results[2]
	if res.Label != "Prowlarr to Radarr" || res.Outcome != wiring.DriftFound || !strings.Contains(res.Detail, "baseUrl") {
		t.Fatalf("want baseUrl drift, got %+v", res)
	}
}

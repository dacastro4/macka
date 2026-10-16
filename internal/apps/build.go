package apps

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/dacastro4/macka/internal/compat"
	"github.com/dacastro4/macka/internal/stack"
	"github.com/dacastro4/macka/internal/wiring"
)

// ReadyTimeout bounds how long macka waits for each app's API.
const ReadyTimeout = 3 * time.Minute

// Plan is the wiring for one stack.
type Plan struct {
	Steps []wiring.Step
	Ready wiring.ReadyFunc
}

// hostURL is how the manager on the host reaches an app's published port.
func hostURL(bind string, port int) string {
	return "http://" + net.JoinHostPort(bind, strconv.Itoa(port))
}

// serviceURL is how apps reach each other inside the Compose network.
func serviceURL(app string, port int) string {
	return "http://" + app + ":" + strconv.Itoa(port)
}

// Build returns the M0 core video stack wiring (core-video-stack spec).
func Build(s *stack.State, sec stack.Secrets, pins compat.Pins) (Plan, error) {
	port := func(app string) (int, error) {
		c, ok := pins[app]
		if !ok {
			return 0, fmt.Errorf("no compatibility record for %s", app)
		}
		return c.Port, nil
	}
	probes := map[string]wiring.Probe{}
	servarr := map[string]*Servarr{}
	names := map[string]struct{ display, api string }{
		"prowlarr": {"Prowlarr", "/api/v1"},
		"radarr":   {"Radarr", "/api/v3"},
		"sonarr":   {"Sonarr", "/api/v3"},
	}
	for _, app := range stack.ServarrApps {
		if !s.Has(app) {
			continue
		}
		p, err := port(app)
		if err != nil {
			return Plan{}, err
		}
		n := names[app]
		c := NewServarr(n.display, hostURL(s.BindAddress, p), n.api, sec.APIKeys[app])
		servarr[app] = c
		probes[app] = c.Ping
	}

	var jf *Jellyfin
	jfPort := 0
	if s.Has("jellyfin") {
		p, err := port("jellyfin")
		if err != nil {
			return Plan{}, err
		}
		jfPort = p
		jf = NewJellyfin(hostURL(s.BindAddress, p), s.Project+"-manager", sec.JellyfinAdminUser, sec.JellyfinAdminPassword)
		probes["jellyfin"] = jf.Ping
	}

	var steps []wiring.Step
	media := []struct {
		app, kind, library, collection string
	}{
		{"radarr", "movies", "Movies", "movies"},
		{"sonarr", "tv", "Shows", "tvshows"},
	}
	for _, m := range media {
		if c := servarr[m.app]; c != nil {
			steps = append(steps, NewRootFolderStep(m.app, c, stack.ContainerPath(m.kind)))
		}
	}
	if pr := servarr["prowlarr"]; pr != nil {
		prPort, _ := port("prowlarr")
		for _, m := range media {
			if servarr[m.app] == nil {
				continue
			}
			p, _ := port(m.app)
			steps = append(steps, NewProwlarrAppStep(pr, m.app, names[m.app].display,
				serviceURL(m.app, p), serviceURL("prowlarr", prPort), sec.APIKeys[m.app]))
		}
	}
	if jf != nil {
		steps = append(steps, NewJellyfinSetupStep(jf, s.Project), NewJellyfinKeyStep(jf))
		for _, m := range media {
			steps = append(steps, NewJellyfinLibraryStep(jf, m.library, m.collection, stack.ContainerPath(m.kind)))
		}
		for _, m := range media {
			if c := servarr[m.app]; c != nil {
				steps = append(steps, NewNotificationStep(m.app, c, jf, "jellyfin", jfPort, JellyfinKeyID))
			}
		}
	}

	ready := func(ctx context.Context, app string) error {
		probe, ok := probes[app]
		if !ok {
			return fmt.Errorf("%s is not part of this stack", app)
		}
		return wiring.WaitReady(ctx, probe, ReadyTimeout, 2*time.Second)
	}
	return Plan{Steps: steps, Ready: ready}, nil
}

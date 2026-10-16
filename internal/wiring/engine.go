// Package wiring runs idempotent check/apply/verify steps (T-4, P-3, P-4).
package wiring

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// LinkState is what Check found on the target app.
type LinkState int

const (
	// Missing means the link is absent and Apply should create it.
	Missing LinkState = iota
	// Satisfied means the link exists with the desired settings.
	Satisfied
	// Drifted means the link exists but differs; M0 never overwrites it (C-3 is M1).
	Drifted
)

// CheckResult is the outcome of a read-only Check.
type CheckResult struct {
	State  LinkState
	Detail string
}

// Step is one wiring link.
type Step interface {
	// ID is unique and stable, e.g. "prowlarr->radarr:app".
	ID() string
	// Label is plain words for the report, e.g. "Prowlarr to Radarr".
	Label() string
	// Targets are apps whose API must be ready before this step runs.
	Targets() []string
	// DependsOn lists step IDs that must succeed first.
	DependsOn() []string
	// Check reads the target API and changes nothing.
	Check(ctx context.Context) (CheckResult, error)
	// Apply creates the link. Called only when Check reports Missing.
	Apply(ctx context.Context) error
	// Verify reads the link back from the target API and changes nothing.
	Verify(ctx context.Context) error
}

// Outcome is what happened to a step.
type Outcome string

const (
	AlreadySatisfied Outcome = "already-satisfied"
	Applied          Outcome = "applied"
	Failed           Outcome = "failed"
	DriftFound       Outcome = "drifted"
	Blocked          Outcome = "blocked"
)

// Result is one step's outcome.
type Result struct {
	StepID  string
	Label   string
	Outcome Outcome
	Detail  string
}

// Connected reports whether the link was verified as existing.
func (r Result) Connected() bool {
	return r.Outcome == AlreadySatisfied || r.Outcome == Applied
}

// Report is the outcome of a run, in execution order.
type Report struct {
	Results []Result
}

// OK reports whether every link is connected.
func (r Report) OK() bool {
	for _, res := range r.Results {
		if !res.Connected() {
			return false
		}
	}
	return true
}

// Count returns how many results have outcome o.
func (r Report) Count(o Outcome) int {
	n := 0
	for _, res := range r.Results {
		if res.Outcome == o {
			n++
		}
	}
	return n
}

// Lines renders one plain-language line per link (W-3 style).
func (r Report) Lines() []string {
	out := make([]string, 0, len(r.Results))
	for _, res := range r.Results {
		switch res.Outcome {
		case AlreadySatisfied, Applied:
			out = append(out, res.Label+": connected")
		case DriftFound:
			out = append(out, res.Label+": changed outside HLI, not overwritten ("+res.Detail+")")
		case Blocked:
			out = append(out, res.Label+": not attempted ("+res.Detail+")")
		default:
			out = append(out, res.Label+": FAILED ("+res.Detail+")")
		}
	}
	return out
}

// ReadyFunc waits until app's API answers, or returns an error on timeout.
type ReadyFunc func(ctx context.Context, app string) error

// Engine runs steps.
type Engine struct {
	Ready ReadyFunc
	// ReadOnly runs Check and Verify only, never Apply (for `hli status`).
	ReadOnly bool
}

// ErrCycle is returned when step dependencies form a cycle.
var ErrCycle = errors.New("wiring steps have a dependency cycle")

// Run executes steps in dependency order. It returns an error only for a
// malformed step list; link failures are recorded in the report.
func (e Engine) Run(ctx context.Context, steps []Step) (Report, error) {
	ordered, err := order(steps)
	if err != nil {
		return Report{}, err
	}
	readiness := map[string]error{}
	done := map[string]Result{}
	var rep Report
	for _, s := range ordered {
		res := e.runOne(ctx, s, readiness, done)
		done[s.ID()] = res
		rep.Results = append(rep.Results, res)
	}
	return rep, nil
}

func (e Engine) runOne(ctx context.Context, s Step, readiness map[string]error, done map[string]Result) Result {
	res := Result{StepID: s.ID(), Label: s.Label()}
	for _, dep := range s.DependsOn() {
		if d := done[dep]; !d.Connected() {
			res.Outcome, res.Detail = Blocked, "needs "+d.Label+" first"
			return res
		}
	}
	for _, app := range s.Targets() {
		err, seen := readiness[app]
		if !seen {
			err = e.waitReady(ctx, app)
			readiness[app] = err
		}
		if err != nil {
			res.Outcome, res.Detail = Blocked, fmt.Sprintf("%s is not answering: %v", app, err)
			return res
		}
	}

	chk, err := s.Check(ctx)
	if err != nil {
		res.Outcome, res.Detail = Failed, "check: "+err.Error()
		return res
	}
	switch chk.State {
	case Drifted:
		res.Outcome, res.Detail = DriftFound, chk.Detail
		return res
	case Missing:
		if e.ReadOnly {
			res.Outcome, res.Detail = Failed, "missing"
			return res
		}
		if err := s.Apply(ctx); err != nil {
			res.Outcome, res.Detail = Failed, "apply: "+err.Error()
			return res
		}
		res.Outcome = Applied
	default:
		res.Outcome = AlreadySatisfied
	}
	// P-3: a link counts only once it is read back from the target app.
	if err := s.Verify(ctx); err != nil {
		res.Outcome, res.Detail = Failed, "verify: "+err.Error()
	}
	return res
}

func (e Engine) waitReady(ctx context.Context, app string) error {
	if e.Ready == nil {
		return nil
	}
	return e.Ready(ctx, app)
}

// order returns steps topologically sorted, keeping input order among peers.
func order(steps []Step) ([]Step, error) {
	byID := make(map[string]Step, len(steps))
	for _, s := range steps {
		if _, dup := byID[s.ID()]; dup {
			return nil, fmt.Errorf("duplicate wiring step id %q", s.ID())
		}
		byID[s.ID()] = s
	}
	const (
		unvisited = iota
		visiting
		visited
	)
	mark := map[string]int{}
	out := make([]Step, 0, len(steps))
	var visit func(s Step, path []string) error
	visit = func(s Step, path []string) error {
		switch mark[s.ID()] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("%w: %s", ErrCycle, strings.Join(append(path, s.ID()), " -> "))
		}
		mark[s.ID()] = visiting
		for _, dep := range s.DependsOn() {
			d, ok := byID[dep]
			if !ok {
				return fmt.Errorf("step %q depends on unknown step %q", s.ID(), dep)
			}
			if err := visit(d, append(path, s.ID())); err != nil {
				return err
			}
		}
		mark[s.ID()] = visited
		out = append(out, s)
		return nil
	}
	for _, s := range steps {
		if err := visit(s, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

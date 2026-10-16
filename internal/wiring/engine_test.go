package wiring

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeStep simulates a link held by a fake target app.
type fakeStep struct {
	id, label  string
	targets    []string
	deps       []string
	present    bool
	drifted    bool
	applyErr   error
	applyNoop  bool // apply "succeeds" but the link never appears
	applyCalls int
	checkCalls int
}

func (f *fakeStep) ID() string          { return f.id }
func (f *fakeStep) Label() string       { return f.label }
func (f *fakeStep) Targets() []string   { return f.targets }
func (f *fakeStep) DependsOn() []string { return f.deps }
func (f *fakeStep) Check(context.Context) (CheckResult, error) {
	f.checkCalls++
	switch {
	case f.drifted:
		return CheckResult{State: Drifted, Detail: "baseUrl differs"}, nil
	case f.present:
		return CheckResult{State: Satisfied}, nil
	}
	return CheckResult{State: Missing}, nil
}
func (f *fakeStep) Apply(context.Context) error {
	f.applyCalls++
	if f.applyErr != nil {
		return f.applyErr
	}
	if !f.applyNoop {
		f.present = true
	}
	return nil
}
func (f *fakeStep) Verify(context.Context) error {
	if !f.present {
		return errors.New("link not found on target")
	}
	return nil
}

func step(id string, deps ...string) *fakeStep {
	return &fakeStep{id: id, label: id, deps: deps}
}

func TestT4_ApplyOnlyWhenMissing(t *testing.T) {
	missing, present := step("a"), step("b")
	present.present = true
	rep, err := Engine{}.Run(context.Background(), []Step{missing, present})
	if err != nil {
		t.Fatal(err)
	}
	if missing.applyCalls != 1 || present.applyCalls != 0 {
		t.Fatalf("apply calls: missing=%d present=%d", missing.applyCalls, present.applyCalls)
	}
	if rep.Results[0].Outcome != Applied || rep.Results[1].Outcome != AlreadySatisfied {
		t.Fatalf("outcomes: %+v", rep.Results)
	}
}

func TestT4_DependencyOrderAndBlocking(t *testing.T) {
	c := step("c", "b")
	b := step("b", "a")
	a := step("a")
	a.applyErr = errors.New("boom")
	rep, err := Engine{}.Run(context.Background(), []Step{c, b, a})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{rep.Results[0].StepID, rep.Results[1].StepID, rep.Results[2].StepID}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("order %v", got)
	}
	if rep.Results[1].Outcome != Blocked || rep.Results[2].Outcome != Blocked {
		t.Fatalf("dependents of a failed step must be blocked: %+v", rep.Results)
	}
	if b.checkCalls != 0 {
		t.Fatal("blocked step must not touch its target")
	}
}

func TestT4_CycleRejected(t *testing.T) {
	_, err := Engine{}.Run(context.Background(), []Step{step("a", "b"), step("b", "a")})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}
}

func TestP3_ApplyWithoutVerifyFails(t *testing.T) {
	s := step("prowlarr->radarr")
	s.label = "Prowlarr to Radarr"
	s.applyNoop = true
	rep, _ := Engine{}.Run(context.Background(), []Step{s})
	r := rep.Results[0]
	if r.Outcome != Failed || !strings.Contains(r.Detail, "verify") {
		t.Fatalf("apply without a read-back link must fail at verify: %+v", r)
	}
	if rep.OK() {
		t.Fatal("report must not be OK")
	}
	line := rep.Lines()[0]
	if !strings.HasPrefix(line, "Prowlarr to Radarr: FAILED") || !strings.Contains(line, "[step prowlarr->radarr]") {
		t.Fatalf("failure line must name the link and the step ID: %q", line)
	}
}

func TestP3_ReportLines(t *testing.T) {
	s := step("x")
	s.label = "Prowlarr to Radarr"
	rep, _ := Engine{}.Run(context.Background(), []Step{s})
	if got := rep.Lines(); len(got) != 1 || got[0] != "Prowlarr to Radarr: connected" {
		t.Fatalf("lines %v", got)
	}
}

func TestP4_SecondRunNoop(t *testing.T) {
	steps := []Step{step("a"), step("b", "a"), step("c", "b")}
	if _, err := (Engine{}).Run(context.Background(), steps); err != nil {
		t.Fatal(err)
	}
	rep, _ := Engine{}.Run(context.Background(), steps)
	if rep.Count(AlreadySatisfied) != 3 {
		t.Fatalf("second run: %+v", rep.Results)
	}
	for _, s := range steps {
		if s.(*fakeStep).applyCalls != 1 {
			t.Fatalf("%s applied %d times across two runs", s.ID(), s.(*fakeStep).applyCalls)
		}
	}
}

func TestP4_DriftNotOverwritten(t *testing.T) {
	s := step("a")
	s.drifted = true
	rep, _ := Engine{}.Run(context.Background(), []Step{s})
	if s.applyCalls != 0 {
		t.Fatal("drifted link must not be overwritten")
	}
	if rep.Results[0].Outcome != DriftFound || rep.OK() {
		t.Fatalf("drift must be reported as a failure: %+v", rep.Results[0])
	}
}

func TestReadinessBlocksTargetsOnce(t *testing.T) {
	calls := map[string]int{}
	eng := Engine{Ready: func(_ context.Context, app string) error {
		calls[app]++
		if app == "jellyfin" {
			return errors.New("timed out after 2m")
		}
		return nil
	}}
	a, b, c := step("a"), step("b"), step("c")
	a.targets = []string{"radarr"}
	b.targets = []string{"jellyfin"}
	c.targets = []string{"jellyfin", "radarr"}
	rep, _ := eng.Run(context.Background(), []Step{a, b, c})
	if rep.Results[0].Outcome != Applied {
		t.Fatalf("ready target: %+v", rep.Results[0])
	}
	for _, r := range rep.Results[1:] {
		if r.Outcome != Blocked || !strings.Contains(r.Detail, "jellyfin is not answering") {
			t.Fatalf("unready target must block: %+v", r)
		}
	}
	if calls["jellyfin"] != 1 || calls["radarr"] != 1 {
		t.Fatalf("readiness must be checked once per app: %v", calls)
	}
}

func TestReadOnlyNeverApplies(t *testing.T) {
	s := step("a")
	rep, _ := Engine{ReadOnly: true}.Run(context.Background(), []Step{s})
	if s.applyCalls != 0 || rep.Results[0].Outcome != Failed {
		t.Fatalf("read-only run: applies=%d %+v", s.applyCalls, rep.Results[0])
	}
}

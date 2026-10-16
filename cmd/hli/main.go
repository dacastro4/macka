// Command hli is the Home Library Installer manager (M0: apply and status).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"hli/internal/apps"
	"hli/internal/compat"
	"hli/internal/platform"
	"hli/internal/redact"
	"hli/internal/stack"
	"hli/internal/wiring"
)

// composer is the slice of Docker Compose the CLI needs.
type composer interface {
	Projects(ctx context.Context) ([]stack.Project, error)
	Up(ctx context.Context, s *stack.State) error
}

type deps struct {
	compose composer
	pins    compat.Pins
	stdout  *redact.Writer
	stderr  *redact.Writer
}

const usage = `usage: hli <command> [-f stack.yml]

commands:
  apply    validate stack.yml, generate the Compose project, start it and wire every app
  status   check every link read-only and print its state
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d := deps{
		compose: platform.Compose{},
		pins:    compat.Default,
		stdout:  redact.NewWriter(os.Stdout),
		stderr:  redact.NewWriter(os.Stderr),
	}
	os.Exit(run(ctx, os.Args[1:], d))
}

// run returns the process exit code: 0 ok, 1 failure, 2 usage.
func run(ctx context.Context, args []string, d deps) int {
	if len(args) == 0 {
		fmt.Fprint(d.stderr, usage)
		return 2
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(d.stderr)
	file := fs.String("f", "stack.yml", "path to stack.yml")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	var err error
	switch cmd {
	case "apply":
		err = apply(ctx, *file, d)
	case "status":
		err = status(ctx, *file, d)
	default:
		fmt.Fprint(d.stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(d.stderr, "hli:", err)
		return 1
	}
	return 0
}

// errLinks means the run finished but not every link is connected.
var errLinks = errors.New("some links are not connected")

func apply(ctx context.Context, file string, d deps) error {
	s, err := stack.Load(file)
	if err != nil {
		return err
	}
	u, err := s.ResolveUser()
	if err != nil {
		return err
	}
	// Render before touching disk so an unpinned image changes nothing.
	gen, err := stack.Generate(s, d.pins, u)
	if err != nil {
		return err
	}
	projects, err := d.compose.Projects(ctx)
	if err != nil {
		return err
	}
	if err := stack.GuardProject(projects, s.Project, s.ComposePath()); err != nil {
		return err
	}
	sec, err := stack.LoadOrCreateSecrets(s)
	if err != nil {
		return err
	}
	d.stdout.Add(sec.Values()...)
	d.stderr.Add(sec.Values()...)

	if err := stack.SeedConfig(s, sec, u); err != nil {
		return err
	}
	if err := stack.WriteGenerated(s, gen); err != nil {
		return err
	}
	fmt.Fprintf(d.stdout, "Starting %d apps in project %q…\n", len(s.Apps), s.Project)
	if err := d.compose.Up(ctx, s); err != nil {
		return err
	}
	return wire(ctx, s, sec, d, false)
}

func status(ctx context.Context, file string, d deps) error {
	s, err := stack.Load(file)
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.SecretsPath()); err != nil {
		return fmt.Errorf("stack has not been applied yet (run `hli apply -f %s`)", file)
	}
	sec, err := stack.LoadOrCreateSecrets(s)
	if err != nil {
		return err
	}
	d.stdout.Add(sec.Values()...)
	d.stderr.Add(sec.Values()...)
	return wire(ctx, s, sec, d, true)
}

func wire(ctx context.Context, s *stack.State, sec stack.Secrets, d deps, readOnly bool) error {
	plan, err := apps.Build(s, sec, d.pins)
	if err != nil {
		return err
	}
	rep, err := wiring.Engine{Ready: plan.Ready, ReadOnly: readOnly}.Run(ctx, plan.Steps)
	if err != nil {
		return err
	}
	printReport(d.stdout, rep)
	if !rep.OK() {
		return errLinks
	}
	return nil
}

func printReport(w io.Writer, rep wiring.Report) {
	fmt.Fprintln(w)
	for _, line := range rep.Lines() {
		fmt.Fprintln(w, "  "+line)
	}
	fmt.Fprintf(w, "\n%d applied, %d already satisfied, %d not connected\n",
		rep.Count(wiring.Applied), rep.Count(wiring.AlreadySatisfied),
		len(rep.Results)-rep.Count(wiring.Applied)-rep.Count(wiring.AlreadySatisfied))
}

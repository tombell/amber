package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	ambercmd "github.com/tombell/amber/internal/cmd"
)

const helpText = `usage: amber [<flags>] <command>

Commands:

  completion    Generate shell completion script
  list          List configured projects or running sessions
  show          Show a session in the Rex app
  start         Start Rex project sessions
  stop          Stop Rex project sessions

Special options:

  -d/--debug                 Show debug logging
  -S/--server <host>         Use a Rex host label or server address
  -C/--client <app>          Show sessions in this Rex app (label or ID)
  -v/--version               Show the version number, then exit
  --help                     Show this message, then exit
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		log.SetFlags(0)
		log.Fatal(err)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("amber", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, helpText) }

	var debug bool
	var version bool
	var server string
	var client string
	flags.BoolVar(&debug, "debug", false, "")
	flags.BoolVar(&debug, "d", false, "")
	flags.BoolVar(&version, "version", false, "")
	flags.BoolVar(&version, "v", false, "")
	targetFlags(flags, &server, &client)

	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if version {
		fmt.Fprintf(stdout, "amber %s (%s)\n", Version, Commit)
		return nil
	}

	remaining := flags.Args()
	if len(remaining) == 0 {
		flags.Usage()
		return flag.ErrHelp
	}

	var logger *log.Logger
	if debug {
		logger = log.New(stderr, "", 0)
	}

	subcommand := func(name string) *flag.FlagSet {
		sub := flag.NewFlagSet("amber "+name, flag.ContinueOnError)
		sub.SetOutput(stderr)
		targetFlags(sub, &server, &client)
		return sub
	}
	app := ambercmd.New(logger, stdout, stderr)

	switch remaining[0] {
	case "__complete":
		if len(remaining) != 2 {
			return errors.New("usage: amber __complete <projects|sessions>")
		}
		return app.WithTarget(server, client).Complete(remaining[1])
	case "completion":
		if len(remaining) != 2 {
			return errors.New("usage: amber completion <bash|fish|zsh>")
		}
		return app.Completion(remaining[1])
	case "list":
		listFlags := subcommand("list")
		var running bool
		listFlags.BoolVar(&running, "running", false, "list running sessions")
		if err := parseFlags(listFlags, remaining[1:]); err != nil {
			return err
		}
		if len(listFlags.Args()) != 0 {
			return errors.New("usage: amber list [--running]")
		}
		app = app.WithTarget(server, client)
		if running {
			return app.ListRunningSessions()
		}
		return app.List()
	case "show":
		showFlags := subcommand("show")
		if err := parseFlags(showFlags, remaining[1:]); err != nil {
			return err
		}
		if len(showFlags.Args()) != 1 {
			return errors.New("usage: amber show <name>")
		}
		return app.WithTarget(server, client).Show(showFlags.Arg(0))
	case "start":
		startFlags := subcommand("start")
		var show bool
		startFlags.BoolVar(&show, "show", false, "show the session after it starts")
		if err := parseFlags(startFlags, remaining[1:]); err != nil {
			return err
		}
		return app.WithTarget(server, client).Start(startFlags.Args(), show)
	case "stop":
		stopFlags := subcommand("stop")
		if err := parseFlags(stopFlags, remaining[1:]); err != nil {
			return err
		}
		return app.WithTarget(server, client).Stop(stopFlags.Args())
	default:
		return fmt.Errorf("%q is not a known command", remaining[0])
	}
}

func targetFlags(flags *flag.FlagSet, server, client *string) {
	serverFlag := nonEmpty{value: server, name: "host label or address"}
	clientFlag := nonEmpty{value: client, name: "app label or ID"}
	flags.Var(serverFlag, "server", "Rex host label or server address")
	flags.Var(serverFlag, "S", "Rex host label or server address")
	flags.Var(clientFlag, "client", "Rex app label or ID")
	flags.Var(clientFlag, "C", "Rex app label or ID")
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	return flags.Parse(args)
}

// nonEmpty is a string flag that rejects an empty value.
type nonEmpty struct {
	value *string
	name  string
}

func (n nonEmpty) String() string {
	if n.value == nil {
		return ""
	}
	return *n.value
}

func (n nonEmpty) Set(value string) error {
	if value == "" {
		return fmt.Errorf("requires a non-empty %s", n.name)
	}
	*n.value = value
	return nil
}

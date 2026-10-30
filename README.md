# Amber

Amber manages [Rex](https://www.superlogical.com/rex/) sessions from YAML files. Describe a project's windows, pane splits, startup commands, and start/stop hooks in `~/.config/amber/<name>.yml`, then bring the whole layout up with `amber start <name>` and tear it down with `amber stop <name>`. Amber checks every config before it touches Rex and creates each session in one request, so a session is built completely or not at all. It works with remote Rex servers too.

## Install

Amber shells out to `rex`. Rex puts `rex` on `PATH` inside its own terminals. Anywhere else, add it to `PATH` yourself or set `REX_BIN_DIR` to its directory. Then install Amber:

```sh
go install github.com/tombell/amber/cmd/amber@latest
```

For a local checkout:

```sh
make dev
```

## Usage

```text
amber list [--running]
amber start <names...>
amber start --show <name>
amber show <name>
amber stop <names...>
amber --server <host> start <names...>
amber --client <app> show <name>
amber completion <bash|fish|zsh>
amber --debug start <names...>
amber --version
```

- `list` prints the configured project names. With `--running`, it prints the labels of every session on the Rex server instead.
- `start` validates every config you name before it touches Rex. It then creates each session that doesn't exist yet. Amber sends a session and all its windows and panes to Rex in one `session.create` request, and Rex either builds all of it or none of it. With `--show` and one project, Amber also switches the Rex app to the new session.
- `show` switches the Rex app to an existing session.
- `stop` runs the `before_stop` hooks, kills the session, then runs the `after_stop` hooks.

Amber finds a project's session by its label. Rex lets two sessions share a label, and when that happens Amber stops with an error. Guessing would risk killing the wrong session.

## Rex servers and apps

`-S`/`--server` takes a host label from `rex hosts` or a server address. Amber passes it to every `rex` command, so the panes start on that server. Amber still reads configs from this machine. The paths in them belong to the server, though, so the project `root` must be absolute or start with `~`.

Amber can't tell where the server's home directory is, and Rex doesn't expand `~` itself. So a pane whose root is under `~` starts in the home directory, and Amber types `cd ~/path` before its commands. You'll see that `cd` in the pane. A path under `~` can't use `..` to climb out of it, or contain a backslash. If a root doesn't exist on the server, Rex starts the pane in the home directory without an error.

Hooks don't work with `--server` yet. They would run on this machine, which is almost never what you want.

`-C`/`--client` picks which Rex app `show` and `start --show` switch, by a label or ID from `rex client ls`. Without it, Rex uses `REX_CLIENT`, or the app if only one is connected.

## Shell completion

For Fish, add this to `~/.config/fish/config.fish`:

```fish
amber completion fish | source
```

For Bash, add `source <(amber completion bash)` to `~/.bashrc`. For Zsh, run `autoload -Uz compinit && compinit`, then add `source <(amber completion zsh)` to `~/.zshrc`.

## Configuration

Create `~/.config/amber/<name>.yml`. The filename without `.yml` becomes the session label. A name is 1 to 64 ASCII letters, digits, `.`, `_`, or `-`.

```yaml
root: ~/Code/acme

before_start:
  - mise install

after_start:
  - echo "acme ready in $REX_SESSION"

before_stop:
  - echo "stopping acme"

windows:
  - label: server
    root: ./cmd/server
    split: right
    ratio: 0.6
    panes:
      - label: app
        commands:
          - go run .
      - root: ../../
        split: down
        panes:
          - commands:
              - go test ./... -count=1
          - commands:
              - tail -f var/app.log
  - label: editor
    commands:
      - nvim .
```

[`examples/project.yml`](examples/project.yml) has a standalone copy.

A project needs a `root` and at least one window. Every window needs a label, and no two windows can share one.

Each window holds one pane, and a pane is either a terminal or a split.

- A terminal takes an optional `label`, `root`, and `commands`. Amber types the commands into the terminal's shell in order, so the shell is still there when they finish.
- A split takes `split: right` or `split: down` and exactly two `panes`. The second pane goes to the right of the first, or below it. `ratio` is the first pane's share of the space, between `0` and `1`, and defaults to `0.5`. Either pane can be another split.

Pane labels let you target panes from the CLI, as in `rex capture -b app`. Rex refuses a label that matches more than one pane, so keep them unique within a session if you plan to do that. As far as I can tell, the Rex app doesn't display pane labels anywhere yet.

Each window and pane inherits its parent's root. A relative path resolves from the parent root, an absolute path replaces it, and `~` is your home directory, or the server's with `--server`.

Rex makes the last window active when it creates a session. Amber focuses the first window afterwards, because that's the one you'd expect from reading the file.

## Hooks

Amber runs each hook with `$SHELL -c` in the project root and stops at the first one that fails. A failing `before_start` hook means Amber never creates the session. A failing `before_stop` hook leaves the session running.

Every hook gets `AMBER_PROJECT` set to the project name. `after_start` and `before_stop` hooks also get `REX_SESSION` set to the project's session ID, so a bare `rex` command in the hook acts on that session. The other hooks get no `REX_SESSION`, since the session doesn't exist yet or is already gone. Amber also removes `REX_BLOCK` from every hook's environment, so a hook can't act on the terminal you ran Amber from by accident.

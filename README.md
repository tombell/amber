# Amber

Amber is a Go CLI for declaratively managing [Rex](https://www.superlogical.com/rex/) sessions. Each `~/.config/amber/<name>.yml` file defines one named Rex session, including its windows, nested pane layouts, commands, and lifecycle hooks.

## Install

Make sure `rex` is available on `PATH` (Rex adds it inside its own terminals), or set `REX_BIN_DIR` to the directory that contains it. Then install Amber:

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

- `list` prints the configured project names. Pass `--running` to print the labels of the server's Rex sessions instead.
- `start` validates every requested config, then creates any session that does not already exist. Each session, with all of its windows and panes, is created in one Rex request, so a failure leaves no half-built session. Pass `--show` with one project to switch the Rex app to it afterwards.
- `show` asks the Rex app to switch to an existing session.
- `stop` runs the project's `before_stop` hooks, kills the session, then runs its `after_stop` hooks.

Amber finds a project's session by its label. Rex labels don't have to be unique, so if more than one session has the project's label, Amber stops with an error instead of guessing.

## Rex servers and apps

`-S`/`--server` takes a host label from `rex hosts` or a server address, and is passed to every `rex` command. Panes then start on that server, so configs used with `--server` must have an absolute project `root` and no `~` paths. Configs are always read locally. Lifecycle hooks are not supported with `--server` yet, because they would run on this machine instead of the server.

`-C`/`--client` chooses the Rex app that `show` and `start --show` switch, from `rex client ls`. Without it, Rex uses `REX_CLIENT` or the only connected app.

## Shell completion

For Fish, add this to `~/.config/fish/config.fish`:

```fish
amber completion fish | source
```

For Bash, use `source <(amber completion bash)` in `~/.bashrc`. For Zsh, run `autoload -Uz compinit && compinit`, then `source <(amber completion zsh)` in `~/.zshrc`.

## Configuration

Create `~/.config/amber/<name>.yml`. The filename without `.yml` becomes the Rex session label. Project names must contain between 1 and 64 ASCII characters using letters, digits, `.`, `_`, or `-`.

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

See [`examples/project.yml`](examples/project.yml) for a standalone example.

Each window is a pane. A pane is either a terminal or a split:

- A **terminal** has an optional `label`, `root`, and `commands`. The commands are typed into the terminal's shell in order, so the shell stays open after they finish.
- A **split** has `split: right` or `split: down` and exactly two `panes`. The second pane goes right of or below the first. The optional `ratio` is the first pane's share, greater than `0` and less than `1`, and defaults to half. Splits can nest.

Roots inherit from project → window → pane, through every nested split. Relative paths resolve from the parent root, absolute paths replace it, and `~` expands to the current user's home directory.

Each project requires a root and at least one window. Window labels are required and must be unique. Rex makes the last window active when it creates a session, so Amber then focuses the first one.

Hooks run through `$SHELL -c` in the project root. Amber sets `AMBER_PROJECT` to the project name. In `after_start` and `before_stop` hooks it also sets `REX_SESSION` to the project's session ID, so `rex` commands in a hook act on that session. In the other hooks, it removes `REX_SESSION`. It always removes `REX_BLOCK`. The project's `before_start` hooks must succeed before the session is created, and its `before_stop` hooks must succeed before the session is killed.

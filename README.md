# Amber

Amber manages [Rex](https://www.superlogical.com/rex/) sessions from YAML files. A project in `~/.config/amber/<name>.yml` lists its sessions, each session lists its tabs, and each tab lays out its panes, startup commands included. `amber start <name>` brings every session up and `amber stop <name>` tears them all down.

Amber checks every config before it touches Rex. If one of a project's sessions fails to start, Amber kills the sessions it already created, so you never end up with half a project. It works with remote Rex servers too.

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
amber show <session>
amber stop <names...>
amber --server <host> start <names...>
amber --client <app> show <session>
amber completion <bash|fish|zsh>
amber --debug start <names...>
amber --version
```

- `list` prints the configured project names. With `--running`, it prints the labels of every session on the Rex server instead.
- `start` validates every config you name, then creates each of the project's sessions that isn't running yet. Each session goes to Rex as one `session.create` request with all its tabs and panes, so Rex builds the whole session or none of it. With `--show` and one project, Amber also switches the Rex app to the project's first session.
- `show` switches the Rex app to a running session. It takes a session label, not a project name.
- `stop` kills each of the project's running sessions and runs the stop hooks around them.

Amber finds a project's sessions by their labels. Rex lets two sessions share a label, and when that happens Amber stops with an error. Guessing would risk killing the wrong session.

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

Create `~/.config/amber/<name>.yml`. The filename without `.yml` is the project name. A project name and a session label are each 1 to 64 ASCII letters, digits, `.`, `_`, or `-`.

```yaml
root: ~/Code/acme

before_start:
  - mise install

after_start:
  - echo "acme ready"

sessions:
  - label: acme-api
    root: ./api
    after_start:
      - echo "api ready in $REX_SESSION"
    tabs:
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
  - label: acme-web
    root: ./web
    tabs:
      - label: dev
        commands:
          - npm run dev
      - label: editor
        commands:
          - nvim .
```

[`examples/project.yml`](examples/project.yml) has a standalone copy.

A project is a group of Rex sessions. A session holds tabs, which Rex calls windows. A tab holds a layout of panes.

A project needs a `root` and at least one session. Every session needs a label and at least one tab. Every tab needs a label. Labels must be unique among their siblings.

The session label is the Rex session label as-is, with no project prefix. That makes it global on the Rex server. If another project already has a running session called `api`, `amber start` treats it as yours and leaves it alone, and `amber stop` kills it. Pick labels that won't collide.

Each tab holds one pane, and a pane is either a terminal or a split.

- A terminal takes an optional `label`, `root`, and `commands`. Amber types the commands into the terminal's shell in order, so the shell is still there when they finish.
- A split takes `split: right` or `split: down` and exactly two `panes`. The second pane goes to the right of the first, or below it. `ratio` is the first pane's share of the space, between `0` and `1`, and defaults to `0.5`. Either pane can be another split.

Pane labels let you target panes from the CLI, as in `rex capture -b app`. Rex refuses a label that matches more than one pane, so keep them unique within a session if you plan to do that. As far as I can tell, the Rex app doesn't display pane labels anywhere yet.

Each session, tab, and pane inherits its parent's root. A relative path resolves from the parent root, an absolute path replaces it, and `~` is your home directory, or the server's with `--server`.

Rex makes the last tab active when it creates a session. Amber focuses the first tab afterwards, because that's the one you'd expect from reading the file.

## Hooks

The project and each session can have `before_start`, `after_start`, `before_stop`, and `after_stop` hooks. Amber runs each hook with `$SHELL -c` in its project or session root and stops at the first one that fails.

`amber start` runs the project's `before_start` hooks once if any session is missing. For each missing session, it runs that session's `before_start` hooks and creates it. Once every session exists, it runs each new session's `after_start` hooks, then the project's. If a `before_start` hook fails, Amber stops there and kills the sessions it already created for the project. A failing `after_start` hook doesn't kill anything, because by then every session is up.

`amber stop` runs the project's `before_stop` hooks first. Then it works through the running sessions in order, running each one's `before_stop` hooks, killing it, and running its `after_stop` hooks. The project's `after_stop` hooks run last. A failing `before_stop` hook leaves that session running, along with every session after it.

Every hook gets `AMBER_PROJECT` set to the project name. A session's `after_start` and `before_stop` hooks also get `REX_SESSION` set to that session's ID, so a bare `rex` command in the hook acts on that session. Project hooks and the other session hooks get no `REX_SESSION`. Amber also removes `REX_BLOCK` from every hook's environment, so a hook can't act on the terminal you ran Amber from by accident.

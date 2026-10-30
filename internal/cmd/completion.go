package cmd

import "fmt"

const bashCompletion = `# bash completion for amber
_amber() {
  local current command candidate i
  local -a server_args

  COMPREPLY=()
  current="${COMP_WORDS[COMP_CWORD]}"
  case "${COMP_WORDS[COMP_CWORD-1]}" in
    -S|--server|-C|--client) return ;;
  esac

  command=""
  server_args=()
  for ((i = 1; i < COMP_CWORD; i++)); do
    case "${COMP_WORDS[i]}" in
      -S|--server) ((i++)); server_args=(--server "${COMP_WORDS[i]}") ;;
      --server=*) server_args=("${COMP_WORDS[i]}") ;;
      -C|--client) ((i++)) ;;
      -v|--version|-h|--help) return ;;
      completion|list|show|start|stop) [[ -z "$command" ]] && command="${COMP_WORDS[i]}" ;;
    esac
  done

  case "$command" in
    "")
      COMPREPLY=( $(compgen -W 'completion list show start stop -d --debug -S --server -C --client -v --version -h --help' -- "$current") )
      ;;
    completion)
      COMPREPLY=( $(compgen -W 'bash fish zsh' -- "$current") )
      ;;
    list)
      COMPREPLY=( $(compgen -W '--running --server' -- "$current") )
      ;;
    show)
      while IFS= read -r candidate; do
        [[ -n "$candidate" && "$candidate" == "$current"* ]] && COMPREPLY+=("$candidate")
      done < <(amber "${server_args[@]}" __complete sessions 2>/dev/null)
      ;;
    start|stop)
      [[ "$command" == start ]] && COMPREPLY=( $(compgen -W '--show' -- "$current") )
      while IFS= read -r candidate; do
        [[ -n "$candidate" && "$candidate" == "$current"* ]] && COMPREPLY+=("$candidate")
      done < <(amber __complete projects 2>/dev/null)
      ;;
  esac
}

complete -F _amber amber
`

const zshCompletion = `#compdef amber

_amber() {
  local command
  local -i i
  local -a server_args candidates

  case "${words[CURRENT-1]}" in
    -S|--server|-C|--client) return ;;
  esac

  command=""
  server_args=()
  for ((i = 2; i < CURRENT; i++)); do
    case "${words[i]}" in
      -S|--server) ((i++)); server_args=(--server "${words[i]}") ;;
      --server=*) server_args=("${words[i]}") ;;
      -C|--client) ((i++)) ;;
      -v|--version|-h|--help) return ;;
      completion|list|show|start|stop) [[ -z "$command" ]] && command="${words[i]}" ;;
    esac
  done

  case "$command" in
    "")
      compadd -- completion list show start stop -d --debug -S --server -C --client -v --version -h --help
      ;;
    completion)
      compadd -- bash fish zsh
      ;;
    list)
      compadd -- --running --server
      ;;
    show)
      candidates=("${(@f)$(amber "${server_args[@]}" __complete sessions 2>/dev/null)}")
      compadd -- "${candidates[@]}"
      ;;
    start|stop)
      candidates=("${(@f)$(amber __complete projects 2>/dev/null)}")
      [[ "$command" == start ]] && candidates+=(--show)
      compadd -- "${candidates[@]}"
      ;;
  esac
}

compdef _amber amber
`

const fishCompletion = `# fish completion for amber
function __amber_server_args
    set -l tokens (commandline -xpc)
    for index in (seq 2 (count $tokens))
        switch $tokens[$index]
            case -S --server
                set -l next (math $index + 1)
                if test $next -le (count $tokens)
                    printf '%s\n' --server $tokens[$next]
                end
            case '--server=*'
                printf '%s\n' $tokens[$index]
        end
    end
end

function __amber_subcommand
    for token in (commandline -xpc)[2..-1]
        if contains -- $token completion list show start stop
            echo $token
            return
        end
    end
end

function __amber_using_subcommand
    set -l command (__amber_subcommand)
    test "$command" = "$argv[1]"
end

function __amber_no_subcommand
    set -l command (__amber_subcommand)
    test -z "$command"
end

function __amber_projects
    set -l used (commandline -xpc)
    for project in (amber __complete projects 2>/dev/null)
        contains -- $project $used; or string escape -- $project
    end
end

function __amber_sessions
    for session in (amber (__amber_server_args) __complete sessions 2>/dev/null)
        string escape -- $session
    end
end

complete -c amber -f

complete -c amber -n __amber_no_subcommand -a completion -d 'Generate shell completion script'
complete -c amber -n __amber_no_subcommand -a list -d 'List configured projects or running sessions'
complete -c amber -n __amber_no_subcommand -a show -d 'Show a session in the Rex app'
complete -c amber -n __amber_no_subcommand -a start -d 'Start Rex project sessions'
complete -c amber -n __amber_no_subcommand -a stop -d 'Stop Rex project sessions'
complete -c amber -n __amber_no_subcommand -s d -l debug -d 'Show debug logging'
complete -c amber -n __amber_no_subcommand -s v -l version -d 'Show the version number'
complete -c amber -n __amber_no_subcommand -s h -l help -d 'Show help'
complete -c amber -s S -l server -r -d 'Rex host label or server address'
complete -c amber -s C -l client -r -d 'Rex app label or ID'

complete -c amber -n '__amber_using_subcommand completion' -a 'bash fish zsh'
complete -c amber -n '__amber_using_subcommand list' -l running -d 'List running sessions'
complete -c amber -n '__amber_using_subcommand show' -a '(__amber_sessions)'
complete -c amber -n '__amber_using_subcommand start' -l show -d 'Show the session after starting'
complete -c amber -n '__amber_using_subcommand start' -a '(__amber_projects)'
complete -c amber -n '__amber_using_subcommand stop' -a '(__amber_projects)'
`

func (a App) Completion(shell string) error {
	var script string
	switch shell {
	case "bash":
		script = bashCompletion
	case "fish":
		script = fishCompletion
	case "zsh":
		script = zshCompletion
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, fish, zsh)", shell)
	}

	_, err := fmt.Fprint(a.stdout, script)
	return err
}

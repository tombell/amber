# bash completion for amber
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

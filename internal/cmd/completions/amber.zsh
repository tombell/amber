#compdef amber

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

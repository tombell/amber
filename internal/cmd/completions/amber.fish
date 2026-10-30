# fish completion for amber
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

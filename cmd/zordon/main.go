// Command zordon summons and commands a fleet of coding agents, each isolated in
// its own git worktree.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ricodevvv/zordon/internal/git"
)

// version is overridden at build time with the release tag.
var version = "dev"

const usage = `Zordon summons and commands a fleet of coding agents, each in its own git worktree.

Usage:
  zordon [command] [flags]

Commands:
  summon <task>     Create a branch and worktree, and start an agent on the task
  list              Show every ranger and the diff it has produced
  logs <ranger>     Print a ranger's output
  attach <ranger>   Attach the terminal to a ranger's session (tmux runner)
  diff <ranger>     Show the patch a ranger has produced
  merge <ranger>    Merge a ranger's branch into its base branch
  stop <ranger>     Terminate a ranger's agent, keeping its branch
  dismiss <ranger>  Stop a ranger and delete its worktree and branch
  agents            List the configured agents
  init              Write a starter .zordon.yml
  version           Print the Zordon version

Run zordon with no command to open the dashboard.
Run zordon <command> -h for the flags of a command.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, git.ErrNotARepository) {
			fmt.Fprintln(os.Stderr, "zordon: not inside a git repository")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "zordon:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return cmdDashboard()
	}

	command, rest := args[0], args[1:]
	switch command {
	case "summon", "s":
		return cmdSummon(rest)
	case "list", "ls", "status":
		return cmdList(rest)
	case "logs", "log":
		return cmdLogs(rest)
	case "attach", "a":
		return cmdAttach(rest)
	case "diff":
		return cmdDiff(rest)
	case "merge":
		return cmdMerge(rest)
	case "stop":
		return cmdStop(rest)
	case "dismiss", "rm":
		return cmdDismiss(rest)
	case "agents":
		return cmdAgents(rest)
	case "init":
		return cmdInit(rest)
	case "version", "--version", "-v":
		fmt.Println("zordon", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		fmt.Print(usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

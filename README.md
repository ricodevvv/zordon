<h1 align="center">Zordon</h1>

<p align="center">
  <b>Summon a fleet of coding agents. Command them from one screen.</b>
</p>

<p align="center">
  <a href="https://github.com/ricodevvv/zordon/actions/workflows/ci.yml"><img src="https://github.com/ricodevvv/zordon/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/ricodevvv/zordon"><img src="https://goreportcard.com/badge/github.com/ricodevvv/zordon" alt="Go report card"></a>
  <a href="https://pkg.go.dev/github.com/ricodevvv/zordon"><img src="https://pkg.go.dev/badge/github.com/ricodevvv/zordon.svg" alt="Go reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT license"></a>
</p>

---

Running one coding agent is easy. Running four is a mess: four terminal tabs, four
branches you forgot to create, and agents overwriting each other in the same working
tree. So you end up running them one at a time, which is the slowest way to use them.

Zordon gives every task its own **git worktree**, its own **branch**, and its own
**agent process** — then puts all of them on one screen.

<p align="center">
  <img src="docs/demo.gif" alt="Three agents working in parallel in the Zordon dashboard, then one of them merged" width="960">
</p>


Agents never share a working tree, so they never collide. You review the diffs side by
side and merge the ones that landed.

## Install

```sh
go install github.com/ricodevvv/zordon/cmd/zordon@latest
```

Or grab a binary from [releases](https://github.com/ricodevvv/zordon/releases). Zordon
needs `git` 2.20+, and `tmux` only if you use the tmux runner.

## Quick start

```sh
cd your-repo
zordon init                              # write .zordon.yml, ignore .zordon/

zordon summon "add a /health endpoint"   # branch + worktree + agent, in one go
zordon summon "fix the flaky auth test"
zordon summon "drop the legacy v1 client"

zordon                                   # the dashboard: all three, live
```

When one of them looks right:

```sh
zordon diff fix-the-flaky-auth-test      # everything it wrote, committed or not
zordon merge --dismiss fix-the-flaky-auth-test
```

`merge` commits whatever the agent left uncommitted, merges the branch into its base,
and with `--dismiss` removes the worktree and branch afterwards.

## How it works

Every ranger is three things Zordon creates and cleans up for you:

| | |
|---|---|
| **branch** | `zordon/<name>`, forked from your default branch |
| **worktree** | `.zordon/worktrees/<name>`, a real checkout the agent works in |
| **process** | your agent's CLI, launched in that directory |

The dashboard polls the worktrees, so the diff you see includes work the agent has not
committed yet — which is most of it, most of the time. Zordon never touches the agent's
index or stages anything behind its back.

State lives in `.zordon/state.json`, so agents outlive the command that started them.
Close the dashboard, come back in an hour, and everyone is still where you left them.

## Commands

| Command | What it does |
|---|---|
| `zordon` | Open the dashboard |
| `zordon summon <task>` | Create a branch and worktree, and start an agent on the task |
| `zordon list` | Every ranger, its status and its diff |
| `zordon logs [-f] [-n N] <ranger>` | Print (or follow) an agent's output |
| `zordon diff <ranger>` | The full patch, uncommitted work included |
| `zordon merge [--squash] [--dismiss] <ranger>` | Land the work on its base branch |
| `zordon attach <ranger>` | Take over an agent's terminal (tmux runner) |
| `zordon stop <ranger>` | Terminate the agent, keep the branch |
| `zordon dismiss [--force] <ranger>` | Stop it and delete its worktree and branch |
| `zordon agents` | The agents you have configured |
| `zordon init` | Write a starter `.zordon.yml` |

`summon` takes `-agent` to pick an agent, `-name` to name the ranger yourself, and
`-base` to fork from something other than the default branch.

## Configuration

`.zordon.yml` at the root of your repository. Every field is optional.

```yaml
agents:
  claude:
    command: ["claude", "-p", "{{task}}"]
    description: "Claude Code, headless prompt mode"
  codex:
    command: ["codex", "exec", "{{task}}"]
  aider:
    command: ["aider", "--yes", "--message", "{{task}}"]
    env:
      AIDER_MODEL: "gpt-5"

default_agent: claude
base_branch: main             # defaults to the repository's default branch
branch_prefix: "zordon/"
worktree_dir: ".zordon/worktrees"
runner: exec                  # exec | tmux
```

`{{task}}` and `{{name}}` are substituted into the command. Commands are argv arrays,
never shell strings, so nothing in a task description can be interpreted as shell.

Anything with a CLI works — including agents that are not coding agents. If it runs in a
directory and edits files, Zordon can command it.

### Runners

**`exec`** (default) detaches the agent and captures its output to a log file. Best for
agents you invoke non-interactively, like `claude -p` or `codex exec`. Read the output
with `zordon logs -f`.

**`tmux`** gives each agent its own tmux session and a real terminal. Use it for agents
you want to talk to; `zordon attach <ranger>` drops you into the live session, and the
output is still mirrored to the log.

## FAQ

**Does this replace my agent?** No. Zordon runs whatever agent CLI you already use. It
owns the branches, the worktrees and the review, not the model.

**What happens to uncommitted work?** It shows up in `zordon diff` and in the dashboard,
and `zordon merge` commits it for you. Nothing is staged or committed until you merge.

**Can two agents work on the same files?** They can, in separate worktrees, and you find
out at merge time — like any two branches. That is the point.

**Where did my disk go?** Each worktree is a full checkout. `zordon dismiss` removes
them; `zordon list` shows what is still around.

## Contributing

Issues and pull requests are welcome. `go test ./...` should pass, and `gofmt -l .`
should print nothing.

## License

[MIT](LICENSE)

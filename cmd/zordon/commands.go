package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ricodevvv/zordon/internal/config"
	"github.com/ricodevvv/zordon/internal/fleet"
	"github.com/ricodevvv/zordon/internal/git"
	"github.com/ricodevvv/zordon/internal/tui"
)

func openFleet() (*fleet.Fleet, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	f, err := fleet.Open(wd)
	if err != nil {
		return nil, err
	}
	return f, f.Refresh()
}

func cmdSummon(args []string) error {
	fs := flag.NewFlagSet("summon", flag.ExitOnError)
	agent := fs.String("agent", "", "agent to run (defaults to default_agent)")
	name := fs.String("name", "", "ranger name (defaults to a slug of the task)")
	base := fs.String("base", "", "branch to fork from (defaults to the repository's default branch)")
	fs.StringVar(agent, "a", "", "shorthand for -agent")
	fs.StringVar(name, "n", "", "shorthand for -name")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: zordon summon [flags] <task>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	task := strings.Join(fs.Args(), " ")
	if task == "" {
		fs.Usage()
		return fmt.Errorf("a task description is required")
	}

	f, err := openFleet()
	if err != nil {
		return err
	}
	ranger, err := f.Summon(fleet.SummonOptions{Task: task, Agent: *agent, Name: *name, Base: *base})
	if err != nil {
		return err
	}

	fmt.Printf("%s summoned %s\n", styleOK.Render("✓"), styleName.Render(ranger.Name))
	fmt.Printf("  agent     %s\n", ranger.Agent)
	fmt.Printf("  branch    %s (from %s)\n", ranger.Branch, ranger.Base)
	fmt.Printf("  worktree  %s\n", relative(f.Repo.Root, ranger.Worktree))
	fmt.Printf("\nWatch it with %s, or open the dashboard with %s.\n",
		styleCmd.Render("zordon logs -f "+ranger.Name), styleCmd.Render("zordon"))
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := openFleet()
	if err != nil {
		return err
	}
	rangers := f.List()
	if len(rangers) == 0 {
		fmt.Printf("No rangers yet. Summon one with %s.\n", styleCmd.Render(`zordon summon "fix the flaky auth test"`))
		return nil
	}
	fmt.Print(renderTable(rangers))
	return nil
}

func cmdLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "follow the log until the agent exits")
	lines := fs.Int("n", 0, "print only the last n lines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	if name == "" {
		return fmt.Errorf("usage: zordon logs [-f] [-n lines] <ranger>")
	}

	f, err := openFleet()
	if err != nil {
		return err
	}
	out, err := f.Logs(name, *lines)
	if err != nil {
		return err
	}
	fmt.Print(out)
	if out != "" && !strings.HasSuffix(out, "\n") {
		fmt.Println()
	}
	if !*follow {
		return nil
	}
	return followLog(f, name, int64(len(out)))
}

func followLog(f *fleet.Fleet, name string, offset int64) error {
	ranger, err := f.Get(name)
	if err != nil {
		return err
	}
	file, err := os.Open(ranger.LogPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
			continue
		}
		if err != nil && err != io.EOF {
			return err
		}
		if err := f.Refresh(); err != nil {
			return err
		}
		if current, err := f.Get(name); err == nil && !current.Status.Active() {
			fmt.Printf("\n%s %s\n", styleDim.Render("ranger finished:"), renderStatus(current.Status))
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func cmdAttach(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zordon attach <ranger>")
	}
	f, err := openFleet()
	if err != nil {
		return err
	}
	return f.Attach(args[0])
}

func cmdDiff(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zordon diff <ranger>")
	}
	f, err := openFleet()
	if err != nil {
		return err
	}
	patch, err := f.Diff(args[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(patch) == "" {
		fmt.Println("No changes yet.")
		return nil
	}
	fmt.Print(patch)
	return nil
}

func cmdMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	squash := fs.Bool("squash", false, "squash the ranger's commits into one")
	dismiss := fs.Bool("dismiss", false, "remove the worktree and branch after merging")
	force := fs.Bool("force", false, "merge even while the agent is still running")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	if name == "" {
		return fmt.Errorf("usage: zordon merge [flags] <ranger>")
	}

	f, err := openFleet()
	if err != nil {
		return err
	}
	ranger, err := f.Get(name)
	if err != nil {
		return err
	}
	if err := f.Merge(name, fleet.MergeOptions{Squash: *squash, Dismiss: *dismiss, Force: *force}); err != nil {
		return err
	}
	fmt.Printf("%s merged %s into %s\n", styleOK.Render("✓"), styleName.Render(ranger.Branch), ranger.Base)
	return nil
}

func cmdStop(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: zordon stop <ranger>")
	}
	f, err := openFleet()
	if err != nil {
		return err
	}
	if err := f.Stop(args[0]); err != nil {
		return err
	}
	fmt.Printf("%s stopped %s\n", styleOK.Render("✓"), styleName.Render(args[0]))
	return nil
}

func cmdDismiss(args []string) error {
	fs := flag.NewFlagSet("dismiss", flag.ExitOnError)
	force := fs.Bool("force", false, "dismiss even while the agent is still running")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	if name == "" {
		return fmt.Errorf("usage: zordon dismiss [--force] <ranger>")
	}

	f, err := openFleet()
	if err != nil {
		return err
	}
	if err := f.Dismiss(name, *force); err != nil {
		return err
	}
	fmt.Printf("%s dismissed %s\n", styleOK.Render("✓"), styleName.Render(name))
	return nil
}

func cmdAgents(args []string) error {
	f, err := openFleet()
	if err != nil {
		return err
	}
	for _, name := range f.Config.AgentNames() {
		agent := f.Config.Agents[name]
		marker := "  "
		if name == f.Config.DefaultAgent {
			marker = styleOK.Render("* ")
		}
		fmt.Printf("%s%s\n", marker, styleName.Render(name))
		fmt.Printf("    command  %s\n", strings.Join(agent.Command, " "))
		if agent.Description != "" {
			fmt.Printf("    %s\n", styleDim.Render(agent.Description))
		}
	}
	if f.Config.Path == "" {
		fmt.Printf("\n%s\n", styleDim.Render("Built-in defaults. Run `zordon init` to customise them."))
	}
	return nil
}

func cmdInit(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := git.Open(wd)
	if err != nil {
		return err
	}
	path, err := config.Default().Write(repo.Root)
	if err != nil {
		return err
	}
	fmt.Printf("%s wrote %s\n", styleOK.Render("✓"), relative(repo.Root, path))

	if added, err := ensureGitignore(repo.Root); err != nil {
		return err
	} else if added {
		fmt.Printf("%s added .zordon/ to .gitignore\n", styleOK.Render("✓"))
	}
	fmt.Printf("\nSummon your first ranger: %s\n", styleCmd.Render(`zordon summon "add a health check endpoint"`))
	return nil
}

func cmdDashboard() error {
	f, err := openFleet()
	if err != nil {
		return err
	}
	return tui.Run(f)
}

func ensureGitignore(root string) (bool, error) {
	path := root + "/.gitignore"
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == ".zordon/" {
			return false, nil
		}
	}
	entry := ".zordon/\n"
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		entry = "\n" + entry
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false, err
	}
	defer file.Close()
	_, err = file.WriteString(entry)
	return err == nil, err
}

func relative(root, path string) string {
	if rel := strings.TrimPrefix(path, root+"/"); rel != path {
		return rel
	}
	return path
}

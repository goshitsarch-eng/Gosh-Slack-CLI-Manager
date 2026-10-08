// Command slackware-cli-manager is a terminal UI for Slackware
// administration.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/app"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/termio"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

const appName = "Slackware CLI Manager"

// version is overridden at release build time with -ldflags "-X main.version=...".
var version = "0.1.1"

func main() {
	runningAsRoot := utils.IsRoot()

	slackVersion, err := slackware.DetectVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
		fmt.Fprintln(os.Stderr, "Proceeding with default settings (Slackware Current)")
		slackVersion = slackware.Version{Kind: slackware.Current}
	} else {
		fmt.Printf("%s v%s\n", appName, version)
		fmt.Printf("Detected: %s\n", slackVersion.DisplayName())
		if !runningAsRoot {
			fmt.Println("Running without root privileges. Mutating actions will be disabled.")
		}
	}

	// Brief pause to show version info
	time.Sleep(500 * time.Millisecond)

	term, err := termio.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	application := app.New(slackVersion, runningAsRoot)
	if err := run(term, application); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nThank you for using %s!\n", appName)
}

// run drives the UI loop. A panic restores the terminal before the trace is
// printed so the shell stays usable.
func run(term *termio.Terminal, a *app.App) (err error) {
	defer func() {
		if r := recover(); r != nil {
			term.Close()
			fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", r, debug.Stack())
			os.Exit(101)
		}
	}()

	tick := time.NewTimer(0)
	defer tick.Stop()

	for {
		term.Draw(a.Render)

		if !tick.Stop() {
			select {
			case <-tick.C:
			default:
			}
		}
		tick.Reset(100 * time.Millisecond)

		select {
		case ev, ok := <-term.Events():
			if !ok {
				term.Close()
				return fmt.Errorf("terminal input closed")
			}
			if ev.Key != nil {
				if m := a.HandleInput(*ev.Key); m != nil {
					a.Update(m)
				}
			}
		case <-tick.C:
		}

		for {
			m, ok := a.TryRecv()
			if !ok {
				break
			}
			a.Update(m)
		}

		if !a.Running {
			break
		}
	}

	term.Close()
	return nil
}

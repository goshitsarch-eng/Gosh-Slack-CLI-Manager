package slackware

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// CommandResult is the outcome of running a command.
type CommandResult struct {
	Success bool
	Stdout  string
	Stderr  string
}

// StreamSource identifies which output stream a line came from.
type StreamSource uint8

const (
	Stdout StreamSource = iota
	Stderr
)

// Executor runs system commands. It is stateless and safe to copy.
type Executor struct{}

// NewExecutor returns a command executor.
func NewExecutor() Executor { return Executor{} }

type streamLine struct {
	source StreamSource
	line   string
}

func readStream(r io.Reader, source StreamSource, out chan<- streamLine, wg *sync.WaitGroup) {
	defer wg.Done()
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 && (err == nil || err == io.EOF) {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			out <- streamLine{source: source, line: line}
		}
		if err != nil {
			return
		}
	}
}

// ExecuteStreaming runs cmd and calls onOutput for each line as it arrives.
// onOutput runs on the calling goroutine.
func (Executor) ExecuteStreaming(cmd string, args []string, onOutput func(StreamSource, string)) CommandResult {
	command := exec.Command(cmd, args...)
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return startFailure(cmd, err)
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return startFailure(cmd, err)
	}
	if err := command.Start(); err != nil {
		return startFailure(cmd, err)
	}

	lines := make(chan streamLine, 64)
	var wg sync.WaitGroup
	wg.Add(2)
	go readStream(stdoutPipe, Stdout, lines, &wg)
	go readStream(stderrPipe, Stderr, lines, &wg)
	go func() {
		wg.Wait()
		close(lines)
	}()

	var stdout, stderr []string
	for l := range lines {
		if onOutput != nil {
			onOutput(l.source, l.line)
		}
		if l.source == Stdout {
			stdout = append(stdout, l.line)
		} else {
			stderr = append(stderr, l.line)
		}
	}

	stdoutText := strings.Join(stdout, "\n")
	stderrText := strings.Join(stderr, "\n")

	if err := command.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return CommandResult{Success: false, Stdout: stdoutText, Stderr: stderrText}
		}
		msg := utils.IOErrorString(err)
		if stderrText != "" {
			msg = stderrText + "\n" + msg
		}
		return CommandResult{Success: false, Stdout: stdoutText, Stderr: msg}
	}
	return CommandResult{Success: true, Stdout: stdoutText, Stderr: stderrText}
}

func startFailure(cmd string, err error) CommandResult {
	return CommandResult{
		Success: false,
		Stderr:  fmt.Sprintf("Unable to start %s: %s", cmd, utils.IOErrorString(err)),
	}
}

// Execute runs a command and collects its output.
func (e Executor) Execute(cmd string, args ...string) CommandResult {
	return e.ExecuteStreaming(cmd, args, nil)
}

// Sbofind searches SlackBuilds with sbofind.
func (e Executor) Sbofind(query string) CommandResult {
	return e.Execute("sbofind", query)
}

// Useradd creates a user with a home directory in the users group.
func (e Executor) Useradd(username string, groups []string, shell string) CommandResult {
	return e.Execute("useradd", "-m", "-g", "users", "-G", strings.Join(groups, ","), "-s", shell, username)
}

// SetPassword sets a user's password through chpasswd.
func (Executor) SetPassword(username, password string) CommandResult {
	command := exec.Command("chpasswd")
	command.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, password))
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return CommandResult{Success: false, Stderr: utils.IOErrorString(err)}
	}
	if err := command.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return CommandResult{Success: false, Stderr: utils.IOErrorString(err)}
		}
		return CommandResult{Success: false, Stdout: stdout.String(), Stderr: stderr.String()}
	}
	return CommandResult{Success: true, Stdout: stdout.String(), Stderr: stderr.String()}
}

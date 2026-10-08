package updater

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestCompGoldenDump(t *testing.T) {
	runCompGolden(t, "updater", func(kind string) goldenComp {
		if strings.HasSuffix(kind, "15") {
			return New(slackware.Version{Kind: slackware.V15_0})
		}
		return New(slackware.Version{Kind: slackware.Current})
	}, func(gc goldenComp, name, arg string) {
		c := gc.(*Component)
		text := goldenText(arg)
		switch name {
		case "start":
			c.startUpdate()
		case "ok":
			c.StepComplete(true, "")
		case "fail":
			c.StepComplete(false, text)
		case "kernel":
			c.SetKernelUpdated(arg == "1")
		case "boot":
			switch arg {
			case "lilo":
				c.bootloader = slackware.BootloaderLilo
			case "grub":
				c.bootloader = slackware.BootloaderGrub
			default:
				c.bootloader = slackware.BootloaderUnknown
			}
		case "confirm":
			c.ConfirmLilo(arg == "1")
		case "out":
			c.AddOutput(text)
		case "bl":
			n, _ := strconv.Atoi(arg)
			c.blacklistEntries = nil
			for i := range n {
				c.blacklistEntries = append(c.blacklistEntries, fmt.Sprintf("pkg%d", i))
			}
		case "cl":
			if arg == "none" {
				c.changelogPath = ""
			} else {
				c.changelogPath = text
			}
		case "new":
			n, _ := strconv.Atoi(arg)
			c.newConfigFiles = nil
			for i := range n {
				c.newConfigFiles = append(c.newConfigFiles, fmt.Sprintf("/etc/dir%d/file%d.conf.new", i, i))
			}
		case "scan":
			c.newConfigFiles = scanNewConfigFiles(arg, 12)
		case "reset":
			c.Reset()
		case "dismiss":
			c.dismissSummary()
		default:
			t.Fatalf("unknown updater op %s", name)
		}
	})
}

// ---- component golden harness (shared by the updater, sbotools, mirror and
// packagesearch packages; compared against the original implementation) ----

type goldenComp interface {
	HandleInput(key tui.KeyEvent) msg.Message
	Render(f *tui.Frame, area tui.Rect)
	HelpText() []msg.KeyHelp
}

func goldenText(arg string) string {
	return strings.ReplaceAll(strings.ReplaceAll(arg, "_", " "), "~", "\n")
}

func goldenKey(t *testing.T, tok string) tui.KeyEvent {
	mods := tui.ModNone
	rest := tok
	if r, ok := strings.CutPrefix(tok, "C-"); ok {
		mods, rest = tui.ModControl, r
	} else if r, ok := strings.CutPrefix(tok, "A-"); ok {
		mods, rest = tui.ModAlt, r
	}
	codes := map[string]tui.KeyCode{
		"Enter": tui.KeyEnter, "Tab": tui.KeyTab, "Esc": tui.KeyEsc, "Backspace": tui.KeyBackspace,
		"Up": tui.KeyUp, "Down": tui.KeyDown,
	}
	if code, ok := codes[rest]; ok {
		return tui.NewKey(code, mods)
	}
	if rest == "Space" {
		return tui.CharKey(' ', mods)
	}
	if utf8.RuneCountInString(rest) == 1 {
		r, _ := utf8.DecodeRuneInString(rest)
		if unicode.IsUpper(r) {
			mods |= tui.ModShift
		}
		return tui.CharKey(r, mods)
	}
	t.Fatalf("unknown key token %q", tok)
	return tui.KeyEvent{}
}

func goldenMsg(m msg.Message) string {
	switch m := m.(type) {
	case msg.StartUpdate:
		return "StartUpdate"
	case msg.ContinueUpdate:
		return "ContinueUpdate"
	case msg.StartSbotoolsInstall:
		return "StartSbotoolsInstall"
	case msg.SetMirror:
		return "SetMirror(" + strconv.Quote(m.URL) + ")"
	case msg.SearchPackages:
		return "SearchPackages(" + strconv.Quote(m.Query) + ")"
	case msg.InstallPackage:
		return "InstallPackage(" + strconv.Quote(m.Name) + ")"
	}
	return fmt.Sprintf("%#v", m)
}

func goldenColor(c tui.Color) string {
	switch c.Kind {
	case tui.ColorKindRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	case tui.ColorKindIndexed:
		return fmt.Sprintf("i%d", c.Index)
	default:
		return "-"
	}
}

// runCompGolden renders every scenario of kind pkg (lines
// name|kind|width|height|ops) when COMP_SCENARIOS and COMP_OUT are set.
func runCompGolden(t *testing.T, pkg string, newComp func(kind string) goldenComp, apply func(c goldenComp, name, arg string)) {
	scenarioPath, outDir := os.Getenv("COMP_SCENARIOS"), os.Getenv("COMP_OUT")
	if scenarioPath == "" || outDir == "" {
		t.Skip("COMP_SCENARIOS/COMP_OUT not set")
	}
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		kind := parts[1]
		if strings.TrimRight(kind, "0123456789") != pkg {
			continue
		}
		width, _ := strconv.Atoi(parts[2])
		height, _ := strconv.Atoi(parts[3])
		ops := ""
		if len(parts) > 4 {
			ops = parts[4]
		}
		c := newComp(kind)
		var msgs []string
		for _, op := range strings.Fields(ops) {
			if k, ok := strings.CutPrefix(op, "k:"); ok {
				if m := c.HandleInput(goldenKey(t, k)); m != nil {
					msgs = append(msgs, goldenMsg(m))
				}
				continue
			}
			name, arg, _ := strings.Cut(op, ":")
			apply(c, name, arg)
		}

		buf := tui.NewBuffer(tui.Rect{Width: width, Height: height})
		c.Render(&tui.Frame{Buf: buf}, buf.Area)
		fmt.Fprintf(&out, "=== %s\n", parts[0])
		fmt.Fprintf(&out, "msgs: %s\n", strings.Join(msgs, ", "))
		for _, h := range c.HelpText() {
			fmt.Fprintf(&out, "help: %s = %s\n", h.Key, h.Desc)
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				out.WriteString(buf.Cell(x, y).Symbol)
			}
			out.WriteString("\n")
		}
		out.WriteString("--- styles\n")
		for y := 0; y < height; y++ {
			var runs []string
			last, n := "", 0
			for x := 0; x < width; x++ {
				cell := buf.Cell(x, y)
				key := fmt.Sprintf("%s/%s/%x", goldenColor(cell.Fg), goldenColor(cell.Bg), uint16(cell.Modifier))
				if n > 0 && key == last {
					n++
					continue
				}
				if n > 0 {
					runs = append(runs, fmt.Sprintf("%d:%s", n, last))
				}
				last, n = key, 1
			}
			if n > 0 {
				runs = append(runs, fmt.Sprintf("%d:%s", n, last))
			}
			fmt.Fprintf(&out, "%d: %s\n", y, strings.Join(runs, " "))
		}
	}
	if err := os.WriteFile(outDir+"/"+pkg+".go.txt", []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

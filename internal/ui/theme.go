// Package ui holds the shared look of the application: the color theme,
// panel helpers, layout helpers and a few composite widgets.
package ui

import (
	"strings"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Theme colors.
var (
	BG              = tui.RGB(12, 16, 24)
	Surface         = tui.RGB(20, 27, 38)
	SurfaceElevated = tui.RGB(28, 36, 48)
	PanelAlt        = tui.RGB(16, 22, 32)
	FG              = tui.RGB(233, 228, 214)
	SoftFG          = tui.RGB(190, 186, 176)
	Accent          = tui.RGB(94, 204, 187)
	AccentAlt       = tui.RGB(235, 178, 74)
	Success         = tui.RGB(116, 204, 149)
	Error           = tui.RGB(224, 101, 84)
	Warning         = tui.RGB(240, 198, 96)
	Muted           = tui.RGB(109, 120, 136)
	Border          = tui.RGB(63, 78, 96)
	BorderFocused   = tui.RGB(94, 204, 187)
)

func style(fg, bg tui.Color) tui.Style { return tui.NewStyle().FG(fg).BG(bg) }

func boldStyle(fg, bg tui.Color) tui.Style { return style(fg, bg).Add(tui.Bold) }

// AppStyle is the base application style.
func AppStyle() tui.Style { return style(FG, BG) }

// DefaultStyle is an alias of AppStyle.
func DefaultStyle() tui.Style { return AppStyle() }

// SurfaceStyle is the panel surface style.
func SurfaceStyle() tui.Style { return style(FG, Surface) }

// SurfaceAltStyle is the alternate panel surface style.
func SurfaceAltStyle() tui.Style { return style(FG, PanelAlt) }

// TitleStyle styles titles.
func TitleStyle() tui.Style { return boldStyle(FG, BG) }

// HeroStyle styles the hero wordmark.
func HeroStyle() tui.Style { return boldStyle(AccentAlt, BG) }

// SubtitleStyle styles subtitles.
func SubtitleStyle() tui.Style { return style(SoftFG, BG) }

// EyebrowStyle styles small section headers.
func EyebrowStyle() tui.Style { return boldStyle(AccentAlt, BG) }

// LabelStyle styles field labels.
func LabelStyle() tui.Style { return style(Muted, BG) }

// ValueStyle styles field values.
func ValueStyle() tui.Style { return style(FG, BG) }

// AccentStyle styles accented text.
func AccentStyle() tui.Style { return style(Accent, BG) }

// HighlightStyle styles highlighted rows.
func HighlightStyle() tui.Style { return boldStyle(FG, SurfaceElevated) }

// TabActive styles the active tab.
func TabActive() tui.Style { return boldStyle(BG, Accent) }

// TabInactive styles inactive tabs.
func TabInactive() tui.Style { return style(SoftFG, Surface) }

// SuccessStyle styles success text.
func SuccessStyle() tui.Style { return style(Success, BG) }

// ErrorStyle styles error text.
func ErrorStyle() tui.Style { return style(Error, BG) }

// WarningStyle styles warnings.
func WarningStyle() tui.Style { return style(Warning, BG) }

// MutedStyle styles secondary text.
func MutedStyle() tui.Style { return style(Muted, BG) }

// StatusBarStyle styles the bottom status bar.
func StatusBarStyle() tui.Style { return style(FG, SurfaceElevated) }

// KeyHint styles key hints.
func KeyHint() tui.Style { return boldStyle(BG, AccentAlt) }

// KeyHintSecondary styles secondary key hints.
func KeyHintSecondary() tui.Style { return boldStyle(FG, Surface) }

// ProgressComplete styles completed steps.
func ProgressComplete() tui.Style { return style(Success, Surface) }

// ProgressPending styles pending steps.
func ProgressPending() tui.Style { return style(Muted, Surface) }

// ProgressRunning styles the running step.
func ProgressRunning() tui.Style { return boldStyle(AccentAlt, Surface) }

// InputActive styles the focused input.
func InputActive() tui.Style { return style(FG, SurfaceElevated) }

// InputInactive styles unfocused inputs.
func InputInactive() tui.Style { return style(SoftFG, Surface) }

// BorderStyle styles panel borders.
func BorderStyle() tui.Style { return style(Border, Surface) }

// BorderFocusedStyle styles focused panel borders.
func BorderFocusedStyle() tui.Style { return style(BorderFocused, Surface) }

// ListSelected styles selected list rows.
func ListSelected() tui.Style { return boldStyle(BG, AccentAlt) }

// BadgeWarning styles warning badges.
func BadgeWarning() tui.Style { return boldStyle(BG, Warning) }

// BadgeSuccess styles success badges.
func BadgeSuccess() tui.Style { return boldStyle(BG, Success) }

// BadgeInfo styles info badges.
func BadgeInfo() tui.Style { return boldStyle(BG, Accent) }

// BadgeNeutral styles neutral badges.
func BadgeNeutral() tui.Style { return boldStyle(FG, SurfaceElevated) }

// Panel is a rounded bordered panel on the surface color.
func Panel(title tui.Line) tui.Block {
	return tui.NewBlock().
		Borders(tui.BordersAll).
		BorderType(tui.BorderRounded).
		BorderStyle(BorderStyle()).
		Style(SurfaceStyle()).
		Title(title)
}

// PanelFocused is a panel with the focused border color.
func PanelFocused(title tui.Line) tui.Block {
	return tui.NewBlock().
		Borders(tui.BordersAll).
		BorderType(tui.BorderRounded).
		BorderStyle(BorderFocusedStyle()).
		Style(SurfaceStyle()).
		Title(title)
}

// PanelAltBlock is a panel on the alternate surface color.
func PanelAltBlock(title tui.Line) tui.Block {
	return tui.NewBlock().
		Borders(tui.BordersAll).
		BorderType(tui.BorderRounded).
		BorderStyle(BorderStyle()).
		Style(SurfaceAltStyle()).
		Title(title)
}

// PanelTitle renders the standard " ● LABEL " panel title.
func PanelTitle(label string) tui.Line {
	return tui.LineFrom(
		tui.Styled(" ", tui.NewStyle().BG(Surface)),
		tui.Styled("● ", tui.NewStyle().FG(Accent).BG(Surface)),
		tui.Styled(strings.ToUpper(label)+" ", tui.NewStyle().FG(AccentAlt).BG(Surface).Add(tui.Bold)),
	)
}

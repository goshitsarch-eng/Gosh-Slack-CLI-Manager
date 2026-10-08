package tui

import (
	"math"
	"strconv"
)

// Gauge is a progress bar filling the whole area, with a centered label.
type Gauge struct {
	block      *Block
	ratio      float64
	label      Span
	hasLabel   bool
	useUnicode bool
	style      Style
	gaugeStyle Style
}

// NewGauge returns an empty gauge (ratio 0).
func NewGauge() Gauge { return Gauge{} }

// Block surrounds the gauge with a block.
func (g Gauge) Block(b Block) Gauge {
	g.block = &b
	return g
}

// Percent sets the ratio from a percentage. It panics when percent is
// outside 0..=100, like the original's assertion.
func (g Gauge) Percent(percent int) Gauge {
	if percent < 0 || percent > 100 {
		panic("Percentage should be between 0 and 100 inclusively.")
	}
	g.ratio = float64(percent) / 100.0
	return g
}

// Ratio sets the filled ratio. It panics when ratio is outside 0..=1, like
// the original's assertion.
func (g Gauge) Ratio(ratio float64) Gauge {
	if !(ratio >= 0 && ratio <= 1) {
		panic("Ratio should be between 0 and 1 inclusively.")
	}
	g.ratio = ratio
	return g
}

// Label sets the label drawn at the center of the gauge.
func (g Gauge) Label(s Span) Gauge {
	g.label, g.hasLabel = s, true
	return g
}

// LabelStr sets an unstyled label.
func (g Gauge) LabelStr(s string) Gauge { return g.Label(Raw(s)) }

// Style sets the base style of the widget area.
func (g Gauge) Style(s Style) Gauge {
	g.style = s
	return g
}

// GaugeStyle sets the style of the bar.
func (g Gauge) GaugeStyle(s Style) Gauge {
	g.gaugeStyle = s
	return g
}

// UseUnicode enables eighth-block symbols for a smoother bar end.
func (g Gauge) UseUnicode(u bool) Gauge {
	g.useUnicode = u
	return g
}

// Render draws the gauge.
func (g Gauge) Render(area Rect, buf *Buffer) {
	buf.SetStyle(area, g.style)
	if g.block != nil {
		g.block.Render(area, buf)
	}
	inner := innerIfSome(g.block, area)
	g.renderGauge(inner, buf)
}

func (g Gauge) renderGauge(area Rect, buf *Buffer) {
	if area.IsEmpty() {
		return
	}
	buf.SetStyle(area, g.gaugeStyle)

	label := g.label
	if !g.hasLabel {
		// Rust formats the rounded f64 with Display, e.g. "50%".
		label = Raw(strconv.FormatFloat(math.Round(g.ratio*100.0), 'f', -1, 64) + "%")
	}
	clampedLabelWidth := min(area.Width, label.Width())
	labelCol := area.Left() + (area.Width-clampedLabelWidth)/2
	labelRow := area.Top() + area.Height/2

	filledWidth := float64(area.Width) * g.ratio
	var end int
	if g.useUnicode {
		end = area.Left() + int(math.Floor(filledWidth))
	} else {
		end = area.Left() + int(math.Round(filledWidth))
	}

	fg, bg := Reset, Reset
	if g.gaugeStyle.HasFg {
		fg = g.gaugeStyle.Fg
	}
	if g.gaugeStyle.HasBg {
		bg = g.gaugeStyle.Bg
	}
	for y := area.Top(); y < area.Bottom(); y++ {
		for x := area.Left(); x < end; x++ {
			c := buf.Cell(x, y)
			if x < labelCol || x > labelCol+clampedLabelWidth || y != labelRow {
				c.SetSymbol(BlockFull)
				c.Fg, c.Bg = fg, bg
			} else {
				c.SetSymbol(" ")
				c.Fg, c.Bg = bg, fg
			}
		}
		if g.useUnicode && g.ratio < 1.0 {
			buf.Cell(end, y).SetSymbol(unicodeBlock(math.Mod(filledWidth, 1.0)))
		}
	}
	buf.SetSpan(labelCol, labelRow, label, clampedLabelWidth)
}

func unicodeBlock(frac float64) string {
	switch int(math.Round(frac * 8.0)) {
	case 1:
		return BlockOneEighth
	case 2:
		return BlockOneQuarter
	case 3:
		return BlockThreeEighths
	case 4:
		return BlockHalf
	case 5:
		return BlockFiveEighths
	case 6:
		return BlockThreeQuarters
	case 7:
		return BlockSevenEighths
	case 8:
		return BlockFull
	default:
		return " "
	}
}

package tui

// Line drawing symbols (ratatui symbols::line).
const (
	SymVertical           = "│"
	SymDoubleVertical     = "║"
	SymThickVertical      = "┃"
	SymHorizontal         = "─"
	SymDoubleHorizontal   = "═"
	SymThickHorizontal    = "━"
	SymTopRight           = "┐"
	SymRoundedTopRight    = "╮"
	SymDoubleTopRight     = "╗"
	SymThickTopRight      = "┓"
	SymTopLeft            = "┌"
	SymRoundedTopLeft     = "╭"
	SymDoubleTopLeft      = "╔"
	SymThickTopLeft       = "┏"
	SymBottomRight        = "┘"
	SymRoundedBottomRight = "╯"
	SymDoubleBottomRight  = "╝"
	SymThickBottomRight   = "┛"
	SymBottomLeft         = "└"
	SymRoundedBottomLeft  = "╰"
	SymDoubleBottomLeft   = "╚"
	SymThickBottomLeft    = "┗"
)

// Block element symbols (ratatui symbols::block).
const (
	BlockFull          = "█"
	BlockSevenEighths  = "▉"
	BlockThreeQuarters = "▊"
	BlockFiveEighths   = "▋"
	BlockHalf          = "▌"
	BlockThreeEighths  = "▍"
	BlockOneQuarter    = "▎"
	BlockOneEighth     = "▏"
)

// Quadrant symbols used by the quadrant border sets.
const (
	quadrantTopLeft                       = "▘"
	quadrantTopRight                      = "▝"
	quadrantBottomLeft                    = "▖"
	quadrantBottomRight                   = "▗"
	quadrantTopHalf                       = "▀"
	quadrantBottomHalf                    = "▄"
	quadrantLeftHalf                      = "▌"
	quadrantRightHalf                     = "▐"
	quadrantTopLeftBottomLeftBottomRight  = "▙"
	quadrantTopLeftTopRightBottomLeft     = "▛"
	quadrantTopLeftTopRightBottomRight    = "▜"
	quadrantTopRightBottomLeftBottomRight = "▟"
)

// BorderSet is the set of symbols used to draw a block border
// (ratatui symbols::border::Set).
type BorderSet struct {
	TopLeft          string
	TopRight         string
	BottomLeft       string
	BottomRight      string
	VerticalLeft     string
	VerticalRight    string
	HorizontalTop    string
	HorizontalBottom string
}

// Border symbol sets.
var (
	BorderSetPlain = BorderSet{
		TopLeft: SymTopLeft, TopRight: SymTopRight,
		BottomLeft: SymBottomLeft, BottomRight: SymBottomRight,
		VerticalLeft: SymVertical, VerticalRight: SymVertical,
		HorizontalTop: SymHorizontal, HorizontalBottom: SymHorizontal,
	}
	BorderSetRounded = BorderSet{
		TopLeft: SymRoundedTopLeft, TopRight: SymRoundedTopRight,
		BottomLeft: SymRoundedBottomLeft, BottomRight: SymRoundedBottomRight,
		VerticalLeft: SymVertical, VerticalRight: SymVertical,
		HorizontalTop: SymHorizontal, HorizontalBottom: SymHorizontal,
	}
	BorderSetDouble = BorderSet{
		TopLeft: SymDoubleTopLeft, TopRight: SymDoubleTopRight,
		BottomLeft: SymDoubleBottomLeft, BottomRight: SymDoubleBottomRight,
		VerticalLeft: SymDoubleVertical, VerticalRight: SymDoubleVertical,
		HorizontalTop: SymDoubleHorizontal, HorizontalBottom: SymDoubleHorizontal,
	}
	BorderSetThick = BorderSet{
		TopLeft: SymThickTopLeft, TopRight: SymThickTopRight,
		BottomLeft: SymThickBottomLeft, BottomRight: SymThickBottomRight,
		VerticalLeft: SymThickVertical, VerticalRight: SymThickVertical,
		HorizontalTop: SymThickHorizontal, HorizontalBottom: SymThickHorizontal,
	}
	BorderSetQuadrantOutside = BorderSet{
		TopLeft:          quadrantTopLeftTopRightBottomLeft,
		TopRight:         quadrantTopLeftTopRightBottomRight,
		BottomLeft:       quadrantTopLeftBottomLeftBottomRight,
		BottomRight:      quadrantTopRightBottomLeftBottomRight,
		VerticalLeft:     quadrantLeftHalf,
		VerticalRight:    quadrantRightHalf,
		HorizontalTop:    quadrantTopHalf,
		HorizontalBottom: quadrantBottomHalf,
	}
	BorderSetQuadrantInside = BorderSet{
		TopRight:         quadrantBottomLeft,
		TopLeft:          quadrantBottomRight,
		BottomRight:      quadrantTopLeft,
		BottomLeft:       quadrantTopRight,
		VerticalLeft:     quadrantRightHalf,
		VerticalRight:    quadrantLeftHalf,
		HorizontalTop:    quadrantBottomHalf,
		HorizontalBottom: quadrantTopHalf,
	}
)

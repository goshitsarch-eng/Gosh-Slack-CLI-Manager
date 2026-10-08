package tui

// Rect is a rectangular area of the terminal. Coordinates follow the u16
// saturating semantics of the original implementation.
type Rect struct {
	X, Y, Width, Height int
}

// NewRect builds a Rect.
func NewRect(x, y, w, h int) Rect { return Rect{X: x, Y: y, Width: w, Height: h} }

func satSub(a, b int) int {
	if a < b {
		return 0
	}
	return a - b
}

// Area returns width*height.
func (r Rect) Area() int { return r.Width * r.Height }

// IsEmpty reports whether the rect has no cells.
func (r Rect) IsEmpty() bool { return r.Width == 0 || r.Height == 0 }

// Left returns the left edge.
func (r Rect) Left() int { return r.X }

// Right returns the exclusive right edge.
func (r Rect) Right() int { return r.X + r.Width }

// Top returns the top edge.
func (r Rect) Top() int { return r.Y }

// Bottom returns the exclusive bottom edge.
func (r Rect) Bottom() int { return r.Y + r.Height }

// Inner shrinks the rect by the given margins, returning a zero rect when it
// does not fit.
func (r Rect) Inner(horizontal, vertical int) Rect {
	if r.Width < 2*horizontal || r.Height < 2*vertical {
		return Rect{}
	}
	return Rect{
		X:      r.X + horizontal,
		Y:      r.Y + vertical,
		Width:  r.Width - 2*horizontal,
		Height: r.Height - 2*vertical,
	}
}

// Intersection returns the overlap of two rects.
func (r Rect) Intersection(o Rect) Rect {
	x1 := max(r.X, o.X)
	y1 := max(r.Y, o.Y)
	x2 := min(r.Right(), o.Right())
	y2 := min(r.Bottom(), o.Bottom())
	return Rect{X: x1, Y: y1, Width: satSub(x2, x1), Height: satSub(y2, y1)}
}

// IndentX moves the left edge right by offset.
func (r Rect) IndentX(offset int) Rect {
	r.X += offset
	r.Width = satSub(r.Width, offset)
	return r
}

package tui

// Clear resets every cell of the area it is rendered into.
type Clear struct{}

// Render resets the cells of area. Cells outside the buffer are ignored
// (the original would panic there).
func (Clear) Render(area Rect, buf *Buffer) {
	area = area.Intersection(buf.Area)
	for x := area.Left(); x < area.Right(); x++ {
		for y := area.Top(); y < area.Bottom(); y++ {
			buf.Cell(x, y).Reset()
		}
	}
}

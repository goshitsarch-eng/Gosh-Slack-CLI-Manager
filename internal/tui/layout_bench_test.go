package tui

import "testing"

func BenchmarkSplitCached(b *testing.B) {
	area := Rect{Width: 140, Height: 45}
	for i := 0; i < b.N; i++ {
		Split(area, Vertical, Length(5), Length(5), Min(10), Length(3))
	}
}

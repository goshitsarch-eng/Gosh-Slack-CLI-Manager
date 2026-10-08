package tui

import (
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui/cassowary"
)

// This file is a port of ratatui 0.29's layout solver (src/layout/layout.rs),
// with a bounded result cache in place of its LRU cache. Rects passed in and returned follow ratatui's u16
// semantics.

// Direction is the axis along which a Layout splits its area.
type Direction uint8

const (
	// Vertical splits the area into rows (ratatui's default direction).
	Vertical Direction = iota
	// Horizontal splits the area into columns.
	Horizontal
)

// ConstraintKind identifies the variant of a layout Constraint.
type ConstraintKind uint8

const (
	ConstraintMin ConstraintKind = iota
	ConstraintMax
	ConstraintLength
	ConstraintPercentage
	ConstraintRatio
	ConstraintFill
)

// Constraint is a ratatui layout constraint. Value holds the u16 value of
// Min/Max/Length/Percentage/Fill, or the u32 numerator of Ratio (Den holds the
// denominator).
type Constraint struct {
	Kind  ConstraintKind
	Value int
	Den   int
}

func clampU16(n int) int { return min(max(n, 0), math.MaxUint16) }
func clampU32(n int) int { return min(max(n, 0), math.MaxUint32) }

// Length requests exactly n cells.
func Length(n int) Constraint { return Constraint{Kind: ConstraintLength, Value: clampU16(n)} }

// Min requests at least n cells.
func Min(n int) Constraint { return Constraint{Kind: ConstraintMin, Value: clampU16(n)} }

// Max requests at most n cells.
func Max(n int) Constraint { return Constraint{Kind: ConstraintMax, Value: clampU16(n)} }

// Percentage requests p percent of the area.
func Percentage(p int) Constraint { return Constraint{Kind: ConstraintPercentage, Value: clampU16(p)} }

// Ratio requests num/den of the area.
func Ratio(num, den int) Constraint {
	return Constraint{Kind: ConstraintRatio, Value: clampU32(num), Den: clampU32(den)}
}

// Fill fills excess space proportionally to w.
func Fill(w int) Constraint { return Constraint{Kind: ConstraintFill, Value: clampU16(w)} }

// Flex controls how excess space is distributed (ratatui's Flex).
// The zero value is FlexStart, which is ratatui's default.
type Flex uint8

const (
	FlexStart Flex = iota
	FlexLegacy
	FlexEnd
	FlexCenter
	FlexSpaceBetween
	FlexSpaceAround
)

// Layout mirrors ratatui's Layout. The zero value (plus Constraints) equals
// ratatui's Layout::default(): vertical, no margin, Flex::Start, spacing 0.
type Layout struct {
	Direction        Direction
	Constraints      []Constraint
	HorizontalMargin int
	VerticalMargin   int
	Flex             Flex
	// Spacing between segments; negative values overlap segments.
	Spacing int
}

// Split splits area like ratatui 0.29's
// Layout::default().direction(dir).constraints(constraints).split(area).
func Split(area Rect, dir Direction, constraints ...Constraint) []Rect {
	return Layout{Direction: dir, Constraints: constraints}.Split(area)
}

// Split splits area into one Rect per constraint.
func (l Layout) Split(area Rect) []Rect {
	segments, _ := l.SplitWithSpacers(area)
	return segments
}

// SplitWithSpacers returns the segment rects and the len(constraints)+1
// spacer rects around them.
func (l Layout) SplitWithSpacers(area Rect) (segments, spacers []Rect) {
	key := l.cacheKey(area)
	layoutCache.mu.Lock()
	cached, ok := layoutCache.entries[key]
	layoutCache.mu.Unlock()
	if ok {
		return slices.Clone(cached.segments), slices.Clone(cached.spacers)
	}

	segments, spacers, err := l.trySplit(area)
	if err != nil {
		panic("failed to split: " + err.Error()) // ratatui: .expect("failed to split")
	}

	layoutCache.mu.Lock()
	if len(layoutCache.entries) >= layoutCacheSize {
		clear(layoutCache.entries)
	}
	layoutCache.entries[key] = cachedSplit{slices.Clone(segments), slices.Clone(spacers)}
	layoutCache.mu.Unlock()
	return segments, spacers
}

// layoutCacheSize bounds the split cache. Like ratatui's layout cache it
// avoids re-running the solver for the same layout every frame.
const layoutCacheSize = 1024

type cachedSplit struct{ segments, spacers []Rect }

var layoutCache = struct {
	mu      sync.Mutex
	entries map[string]cachedSplit
}{entries: make(map[string]cachedSplit)}

func (l Layout) cacheKey(area Rect) string {
	b := make([]byte, 0, 64+len(l.Constraints)*12)
	b = fmt.Appendf(b, "%d,%d,%d,%d|%d|%d,%d|%d|%d|", area.X, area.Y, area.Width, area.Height,
		l.Direction, l.HorizontalMargin, l.VerticalMargin, l.Flex, l.Spacing)
	for _, c := range l.Constraints {
		b = fmt.Appendf(b, "%d:%d:%d;", c.Kind, c.Value, c.Den)
	}
	return string(b)
}

// floatPrecisionMultiplier decides floating point precision when rounding.
const floatPrecisionMultiplier = 100.0

// Strengths (ratatui layout::strengths).
const (
	spacerSizeEq     = cassowary.Required / 10.0
	minSizeGe        = cassowary.Strong * 100.0
	maxSizeLe        = cassowary.Strong * 100.0
	lengthSizeEq     = cassowary.Strong * 10.0
	percentageSizeEq = cassowary.Strong
	ratioSizeEq      = cassowary.Strong / 10.0
	minSizeEq        = cassowary.Medium * 10.0
	maxSizeEq        = cassowary.Medium * 10.0
	fillGrow         = cassowary.Medium
	grow             = cassowary.Medium / 10.0
	spaceGrow        = cassowary.Weak * 10.0
	allSegmentGrow   = cassowary.Weak
)

// u16 helpers mirroring Rust's saturating arithmetic.
func u16(n int) int          { return clampU16(n) }
func satAddU16(a, b int) int { return clampU16(a + b) }
func satSubU16(a, b int) int { return clampU16(a - b) }

// innerArea is ratatui's Rect::inner(Margin).
func innerArea(r Rect, horizontal, vertical int) Rect {
	h, v := u16(horizontal), u16(vertical)
	dh, dv := clampU16(h*2), clampU16(v*2)
	w, ht := u16(r.Width), u16(r.Height)
	if w < dh || ht < dv {
		return Rect{}
	}
	return Rect{
		X:      satAddU16(u16(r.X), h),
		Y:      satAddU16(u16(r.Y), v),
		Width:  satSubU16(w, dh),
		Height: satSubU16(ht, dv),
	}
}

// spacingValue mirrors ratatui's Spacing (from i32) converted to i16.
func spacingValue(s int) int16 {
	s = min(max(s, math.MinInt16), math.MaxInt16)
	if s < 0 {
		return -int16(uint16(-s)) // Overlap(x) => -(x as i16)
	}
	return int16(uint16(s)) // Space(x) => x as i16
}

type element struct {
	start, end cassowary.Variable
}

func (e element) size() cassowary.Expression { return e.end.Sub(e.start) }

func (e element) hasMaxSize(size int, strength float64) *cassowary.Constraint {
	return cassowary.Le(e.size(), strength, cassowary.Const(float64(size)*floatPrecisionMultiplier))
}

func (e element) hasMinSize(size int16, strength float64) *cassowary.Constraint {
	return cassowary.Ge(e.size(), strength, cassowary.Const(float64(size)*floatPrecisionMultiplier))
}

func (e element) hasIntSize(size int, strength float64) *cassowary.Constraint {
	return cassowary.Eq(e.size(), strength, cassowary.Const(float64(size)*floatPrecisionMultiplier))
}

func (e element) hasSize(size cassowary.Expression, strength float64) *cassowary.Constraint {
	return cassowary.Eq(e.size(), strength, size)
}

func (e element) isEmpty() *cassowary.Constraint {
	return cassowary.Eq(e.size(), cassowary.Required-1.0, cassowary.Const(0))
}

func (l Layout) trySplit(area Rect) ([]Rect, []Rect, error) {
	solver := cassowary.NewSolver()

	inner := innerArea(area, l.HorizontalMargin, l.VerticalMargin)
	var areaStart, areaEnd float64
	if l.Direction == Horizontal {
		areaStart = float64(inner.X) * floatPrecisionMultiplier
		areaEnd = float64(satAddU16(inner.X, inner.Width)) * floatPrecisionMultiplier
	} else {
		areaStart = float64(inner.Y) * floatPrecisionMultiplier
		areaEnd = float64(satAddU16(inner.Y, inner.Height)) * floatPrecisionMultiplier
	}

	variableCount := len(l.Constraints)*2 + 2
	variables := make([]cassowary.Variable, variableCount)
	for i := range variables {
		variables[i] = cassowary.NewVariable()
	}
	spacers := make([]element, 0, len(l.Constraints)+1)
	for i := 0; i+1 < len(variables); i += 2 {
		spacers = append(spacers, element{variables[i], variables[i+1]})
	}
	segments := make([]element, 0, len(l.Constraints))
	for i := 1; i+1 < len(variables); i += 2 {
		segments = append(segments, element{variables[i], variables[i+1]})
	}

	flex := l.Flex
	spacing := spacingValue(l.Spacing)
	areaSize := element{variables[0], variables[len(variables)-1]}

	steps := []func() error{
		func() error { return configureArea(solver, areaSize, areaStart, areaEnd) },
		func() error { return configureVariableInAreaConstraints(solver, variables, areaSize) },
		func() error { return configureVariableConstraints(solver, variables) },
		func() error { return configureFlexConstraints(solver, areaSize, spacers, flex, spacing) },
		func() error { return configureConstraints(solver, areaSize, segments, l.Constraints, flex) },
		func() error { return configureFillConstraints(solver, segments, l.Constraints, flex) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, nil, err
		}
	}
	if flex != FlexLegacy {
		for i := 0; i+1 < len(segments); i++ {
			if err := solver.AddConstraint(segments[i].hasSize(segments[i+1].size(), allSegmentGrow)); err != nil {
				return nil, nil, err
			}
		}
	}

	changes := map[cassowary.Variable]float64{}
	for _, c := range solver.FetchChanges() {
		changes[c.Variable] = c.Value
	}

	return changesToRects(changes, segments, inner, l.Direction),
		changesToRects(changes, spacers, inner, l.Direction), nil
}

func configureArea(s *cassowary.Solver, area element, start, end float64) error {
	if err := s.AddConstraint(cassowary.Eq(area.start.Expr(), cassowary.Required, cassowary.Const(start))); err != nil {
		return err
	}
	return s.AddConstraint(cassowary.Eq(area.end.Expr(), cassowary.Required, cassowary.Const(end)))
}

func configureVariableInAreaConstraints(s *cassowary.Solver, variables []cassowary.Variable, area element) error {
	for _, v := range variables {
		if err := s.AddConstraint(cassowary.Ge(v.Expr(), cassowary.Required, area.start.Expr())); err != nil {
			return err
		}
		if err := s.AddConstraint(cassowary.Le(v.Expr(), cassowary.Required, area.end.Expr())); err != nil {
			return err
		}
	}
	return nil
}

func configureVariableConstraints(s *cassowary.Solver, variables []cassowary.Variable) error {
	// variables.iter().skip(1).tuples(): (v1,v2), (v3,v4), ...
	for i := 1; i+1 < len(variables); i += 2 {
		if err := s.AddConstraint(cassowary.Le(variables[i].Expr(), cassowary.Required, variables[i+1].Expr())); err != nil {
			return err
		}
	}
	return nil
}

func configureConstraints(s *cassowary.Solver, area element, segments []element, constraints []Constraint, flex Flex) error {
	for i := 0; i < len(constraints) && i < len(segments); i++ {
		c, seg := constraints[i], segments[i]
		var cs []*cassowary.Constraint
		switch c.Kind {
		case ConstraintMax:
			cs = append(cs, seg.hasMaxSize(c.Value, maxSizeLe), seg.hasIntSize(c.Value, maxSizeEq))
		case ConstraintMin:
			cs = append(cs, seg.hasMinSize(int16(uint16(c.Value)), minSizeGe))
			if flex == FlexLegacy {
				cs = append(cs, seg.hasIntSize(c.Value, minSizeEq))
			} else {
				cs = append(cs, seg.hasSize(area.size(), fillGrow))
			}
		case ConstraintLength:
			cs = append(cs, seg.hasIntSize(c.Value, lengthSizeEq))
		case ConstraintPercentage:
			size := area.size().Mul(float64(c.Value)).Div(100.00)
			cs = append(cs, seg.hasSize(size, percentageSizeEq))
		case ConstraintRatio:
			size := area.size().Mul(float64(c.Value)).Div(float64(max(c.Den, 1)))
			cs = append(cs, seg.hasSize(size, ratioSizeEq))
		case ConstraintFill:
			cs = append(cs, seg.hasSize(area.size(), fillGrow))
		}
		for _, cn := range cs {
			if err := s.AddConstraint(cn); err != nil {
				return err
			}
		}
	}
	return nil
}

func configureFlexConstraints(s *cassowary.Solver, area element, spacers []element, flex Flex, spacing int16) error {
	var inner []element
	if len(spacers) >= 2 {
		inner = spacers[1 : len(spacers)-1]
	}
	spacingF := float64(spacing) * floatPrecisionMultiplier
	add := func(c *cassowary.Constraint) error { return s.AddConstraint(c) }
	first, last := spacers[0], spacers[len(spacers)-1]

	switch flex {
	case FlexLegacy:
		for _, sp := range inner {
			if err := add(sp.hasSize(cassowary.Const(spacingF), spacerSizeEq)); err != nil {
				return err
			}
		}
		if err := add(first.isEmpty()); err != nil {
			return err
		}
		return add(last.isEmpty())
	case FlexSpaceAround:
		for i := 0; i < len(spacers); i++ {
			for j := i + 1; j < len(spacers); j++ {
				if err := add(spacers[i].hasSize(spacers[j].size(), spacerSizeEq)); err != nil {
					return err
				}
			}
		}
		for _, sp := range spacers {
			if err := add(sp.hasMinSize(spacing, spacerSizeEq)); err != nil {
				return err
			}
			if err := add(sp.hasSize(area.size(), spaceGrow)); err != nil {
				return err
			}
		}
	case FlexSpaceBetween:
		for i := 0; i < len(inner); i++ {
			for j := i + 1; j < len(inner); j++ {
				if err := add(inner[i].hasSize(inner[j].size(), spacerSizeEq)); err != nil {
					return err
				}
			}
		}
		for _, sp := range inner {
			if err := add(sp.hasMinSize(spacing, spacerSizeEq)); err != nil {
				return err
			}
			if err := add(sp.hasSize(area.size(), spaceGrow)); err != nil {
				return err
			}
		}
		if err := add(first.isEmpty()); err != nil {
			return err
		}
		return add(last.isEmpty())
	case FlexStart:
		for _, sp := range inner {
			if err := add(sp.hasSize(cassowary.Const(spacingF), spacerSizeEq)); err != nil {
				return err
			}
		}
		if err := add(first.isEmpty()); err != nil {
			return err
		}
		return add(last.hasSize(area.size(), grow))
	case FlexCenter:
		for _, sp := range inner {
			if err := add(sp.hasSize(cassowary.Const(spacingF), spacerSizeEq)); err != nil {
				return err
			}
		}
		if err := add(first.hasSize(area.size(), grow)); err != nil {
			return err
		}
		if err := add(last.hasSize(area.size(), grow)); err != nil {
			return err
		}
		return add(first.hasSize(last.size(), spacerSizeEq))
	case FlexEnd:
		for _, sp := range inner {
			if err := add(sp.hasSize(cassowary.Const(spacingF), spacerSizeEq)); err != nil {
				return err
			}
		}
		if err := add(last.isEmpty()); err != nil {
			return err
		}
		return add(first.hasSize(area.size(), grow))
	}
	return nil
}

func configureFillConstraints(s *cassowary.Solver, segments []element, constraints []Constraint, flex Flex) error {
	type item struct {
		c   Constraint
		seg element
	}
	var items []item
	for i := 0; i < len(constraints) && i < len(segments); i++ {
		c := constraints[i]
		if c.Kind == ConstraintFill || (flex != FlexLegacy && c.Kind == ConstraintMin) {
			items = append(items, item{c, segments[i]})
		}
	}
	scaling := func(c Constraint) float64 {
		if c.Kind == ConstraintFill {
			return math.Max(float64(c.Value), 1e-6)
		}
		return 1.0
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			left, right := items[i], items[j]
			ls, rs := scaling(left.c), scaling(right.c)
			cn := cassowary.Eq(left.seg.size().Mul(rs), grow, right.seg.size().Mul(ls))
			if err := s.AddConstraint(cn); err != nil {
				return err
			}
		}
	}
	return nil
}

// f64ToU16 mirrors Rust's saturating `as u16` cast.
func f64ToU16(f float64) int {
	if math.IsNaN(f) || f <= 0 {
		return 0
	}
	if f >= math.MaxUint16 {
		return math.MaxUint16
	}
	return int(f)
}

func changesToRects(changes map[cassowary.Variable]float64, elements []element, area Rect, dir Direction) []Rect {
	rects := make([]Rect, len(elements))
	for i, e := range elements {
		start := changes[e.start]
		end := changes[e.end]
		s := f64ToU16(math.Round(math.Round(start) / floatPrecisionMultiplier))
		en := f64ToU16(math.Round(math.Round(end) / floatPrecisionMultiplier))
		size := satSubU16(en, s)
		if dir == Horizontal {
			rects[i] = Rect{X: s, Y: area.Y, Width: size, Height: area.Height}
		} else {
			rects[i] = Rect{X: area.X, Y: s, Width: area.Width, Height: size}
		}
	}
	return rects
}

package cassowary

import "sort"

type symbolType uint8

const (
	symInvalid symbolType = iota
	symExternal
	symSlack
	symError
	symDummy
)

// symbol ids are unique across all types within a solver (they share the
// id_tick counter), so ordering by id alone is a total order.
type symbol struct {
	id  int
	typ symbolType
}

func invalidSymbol() symbol { return symbol{0, symInvalid} }

func nearZero(v float64) bool {
	const eps = 1e-8
	if v < 0 {
		return -v < eps
	}
	return v < eps
}

type cell struct {
	sym symbol
	val float64
}

// row is a linear expression over symbols. Cells are kept sorted by symbol id,
// which defines the (deterministic) iteration order.
type row struct {
	cells    []cell
	constant float64
}

func newRow(constant float64) *row { return &row{constant: constant} }

func (r *row) clone() *row {
	return &row{cells: append([]cell(nil), r.cells...), constant: r.constant}
}

func (r *row) find(s symbol) (int, bool) {
	i := sort.Search(len(r.cells), func(i int) bool { return r.cells[i].sym.id >= s.id })
	return i, i < len(r.cells) && r.cells[i].sym == s
}

func (r *row) add(v float64) float64 {
	r.constant += v
	return r.constant
}

func (r *row) insertSymbol(s symbol, coefficient float64) {
	i, ok := r.find(s)
	if !ok {
		if !nearZero(coefficient) {
			r.cells = append(r.cells, cell{})
			copy(r.cells[i+1:], r.cells[i:])
			r.cells[i] = cell{s, coefficient}
		}
		return
	}
	r.cells[i].val += coefficient
	if nearZero(r.cells[i].val) {
		r.cells = append(r.cells[:i], r.cells[i+1:]...)
	}
}

func (r *row) insertRow(other *row, coefficient float64) bool {
	constantDiff := other.constant * coefficient
	r.constant += constantDiff
	for _, c := range other.cells {
		r.insertSymbol(c.sym, c.val*coefficient)
	}
	return constantDiff != 0
}

// take removes s, returning its coefficient and whether it was present.
func (r *row) take(s symbol) (float64, bool) {
	i, ok := r.find(s)
	if !ok {
		return 0, false
	}
	v := r.cells[i].val
	r.cells = append(r.cells[:i], r.cells[i+1:]...)
	return v, true
}

func (r *row) remove(s symbol) { r.take(s) }

func (r *row) reverseSign() {
	r.constant = -r.constant
	for i := range r.cells {
		r.cells[i].val = -r.cells[i].val
	}
}

func (r *row) solveForSymbol(s symbol) {
	v, ok := r.take(s)
	if !ok {
		panic("cassowary: solveForSymbol: symbol not in row")
	}
	coeff := -1.0 / v
	r.constant *= coeff
	for i := range r.cells {
		r.cells[i].val *= coeff
	}
}

func (r *row) solveForSymbols(lhs, rhs symbol) {
	r.insertSymbol(lhs, -1.0)
	r.solveForSymbol(rhs)
}

func (r *row) coefficientFor(s symbol) float64 {
	if i, ok := r.find(s); ok {
		return r.cells[i].val
	}
	return 0
}

func (r *row) substitute(s symbol, other *row) bool {
	if coeff, ok := r.take(s); ok {
		return r.insertRow(other, coeff)
	}
	return false
}

// rowMap is the tableau: basic symbol -> row, iterated in ascending symbol id.
type rowMap struct {
	keys []symbol
	m    map[symbol]*row
}

func newRowMap() rowMap { return rowMap{m: map[symbol]*row{}} }

func (rm *rowMap) get(s symbol) (*row, bool) {
	r, ok := rm.m[s]
	return r, ok
}

func (rm *rowMap) insert(s symbol, r *row) {
	if _, ok := rm.m[s]; !ok {
		i := sort.Search(len(rm.keys), func(i int) bool { return rm.keys[i].id >= s.id })
		rm.keys = append(rm.keys, symbol{})
		copy(rm.keys[i+1:], rm.keys[i:])
		rm.keys[i] = s
	}
	rm.m[s] = r
}

func (rm *rowMap) remove(s symbol) (*row, bool) {
	r, ok := rm.m[s]
	if !ok {
		return nil, false
	}
	delete(rm.m, s)
	i := sort.Search(len(rm.keys), func(i int) bool { return rm.keys[i].id >= s.id })
	rm.keys = append(rm.keys[:i], rm.keys[i+1:]...)
	return r, true
}

func (rm *rowMap) clear() {
	rm.keys = rm.keys[:0]
	rm.m = map[symbol]*row{}
}

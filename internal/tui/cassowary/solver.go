package cassowary

import (
	"math"
	"sort"
)

type tag struct {
	marker symbol
	other  symbol
}

type editInfo struct {
	tag        tag
	constraint *Constraint
	constant   float64
}

type varData struct {
	value  float64
	symbol symbol
	count  int
}

// Change is a variable together with its new value, as reported by
// FetchChanges.
type Change struct {
	Variable Variable
	Value    float64
}

// Solver is a constraint solver using the Cassowary algorithm.
type Solver struct {
	cns                map[*Constraint]tag
	varData            map[Variable]*varData
	varForSymbol       map[symbol]Variable
	publicChanges      []Change
	changed            map[Variable]struct{}
	shouldClearChanges bool
	rows               rowMap
	edits              map[Variable]*editInfo
	infeasibleRows     []symbol // never contains external symbols
	objective          *row
	artificial         *row
	idTick             int
}

// NewSolver constructs a new solver.
func NewSolver() *Solver {
	return &Solver{
		cns:          map[*Constraint]tag{},
		varData:      map[Variable]*varData{},
		varForSymbol: map[symbol]Variable{},
		changed:      map[Variable]struct{}{},
		rows:         newRowMap(),
		edits:        map[Variable]*editInfo{},
		objective:    newRow(0),
		idTick:       1,
	}
}

// AddConstraints adds several constraints, stopping at the first error.
func (s *Solver) AddConstraints(cs ...*Constraint) error {
	for _, c := range cs {
		if err := s.AddConstraint(c); err != nil {
			return err
		}
	}
	return nil
}

// AddConstraint adds a constraint to the solver.
func (s *Solver) AddConstraint(c *Constraint) error {
	if _, ok := s.cns[c]; ok {
		return ErrDuplicateConstraint
	}

	r, t := s.createRow(c)
	subject := chooseSubject(r, t)

	if subject.typ == symInvalid && allDummies(r) {
		if !nearZero(r.constant) {
			return ErrUnsatisfiableConstraint
		}
		subject = t.marker
	}

	if subject.typ == symInvalid {
		ok, err := s.addWithArtificialVariable(r)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnsatisfiableConstraint
		}
	} else {
		r.solveForSymbol(subject)
		s.substitute(subject, r)
		if subject.typ == symExternal && r.constant != 0 {
			s.varChanged(s.varForSymbol[subject])
		}
		s.rows.insert(subject, r)
	}

	s.cns[c] = t

	return s.optimise(s.objective)
}

// RemoveConstraint removes a constraint from the solver.
func (s *Solver) RemoveConstraint(c *Constraint) error {
	t, ok := s.cns[c]
	if !ok {
		return ErrUnknownConstraint
	}
	delete(s.cns, c)

	s.removeConstraintEffects(c, t)

	if _, ok := s.rows.remove(t.marker); !ok {
		leaving, r, found := s.getMarkerLeavingRow(t.marker)
		if !found {
			return InternalSolverError("Failed to find leaving row.")
		}
		r.solveForSymbols(leaving, t.marker)
		s.substitute(t.marker, r)
	}

	if err := s.optimise(s.objective); err != nil {
		return err
	}

	for _, term := range c.expr.Terms {
		if !nearZero(term.Coefficient) {
			if vd, ok := s.varData[term.Variable]; ok {
				vd.count--
				if vd.count == 0 {
					delete(s.varForSymbol, vd.symbol)
					delete(s.varData, term.Variable)
				}
			}
		}
	}
	return nil
}

// HasConstraint reports whether a constraint has been added to the solver.
func (s *Solver) HasConstraint(c *Constraint) bool {
	_, ok := s.cns[c]
	return ok
}

// AddEditVariable adds an edit variable to the solver.
func (s *Solver) AddEditVariable(v Variable, strength float64) error {
	if _, ok := s.edits[v]; ok {
		return ErrDuplicateEditVariable
	}
	strength = ClipStrength(strength)
	if strength == Required {
		return ErrBadRequiredStrength
	}
	cn := NewConstraint(Expression{Terms: []Term{{v, 1}}}, Equal, strength)
	if err := s.AddConstraint(cn); err != nil {
		panic(err) // Rust: .unwrap()
	}
	s.edits[v] = &editInfo{tag: s.cns[cn], constraint: cn, constant: 0}
	return nil
}

// RemoveEditVariable removes an edit variable from the solver.
func (s *Solver) RemoveEditVariable(v Variable) error {
	info, ok := s.edits[v]
	if !ok {
		return ErrUnknownEditVariable
	}
	delete(s.edits, v)
	if err := s.RemoveConstraint(info.constraint); err != nil {
		if err == ErrUnknownConstraint {
			return InternalSolverError("Edit constraint not in system")
		}
		return err
	}
	return nil
}

// HasEditVariable reports whether v is an edit variable.
func (s *Solver) HasEditVariable(v Variable) bool {
	_, ok := s.edits[v]
	return ok
}

// SuggestValue suggests a value for the given edit variable.
func (s *Solver) SuggestValue(v Variable, value float64) error {
	info, ok := s.edits[v]
	if !ok {
		return ErrUnknownEditVariable
	}
	delta := value - info.constant
	info.constant = value
	marker, other := info.tag.marker, info.tag.other

	if r, ok := s.rows.get(marker); ok {
		if r.add(-delta) < 0 {
			s.infeasibleRows = append(s.infeasibleRows, marker)
		}
	} else if r, ok := s.rows.get(other); ok {
		if r.add(delta) < 0 {
			s.infeasibleRows = append(s.infeasibleRows, other)
		}
	} else {
		for _, sym := range s.rows.keys {
			r := s.rows.m[sym]
			coeff := r.coefficientFor(marker)
			diff := delta * coeff
			if diff != 0 && sym.typ == symExternal {
				s.varChanged(s.varForSymbol[sym])
			}
			if coeff != 0 && r.add(diff) < 0 && sym.typ != symExternal {
				s.infeasibleRows = append(s.infeasibleRows, sym)
			}
		}
	}
	return s.dualOptimise()
}

func (s *Solver) varChanged(v Variable) {
	if s.shouldClearChanges {
		clear(s.changed)
		s.shouldClearChanges = false
	}
	s.changed[v] = struct{}{}
}

// FetchChanges returns all changes to the values of variables since the last
// call to this function. Variables start with a NaN "previous value", so the
// first fetch reports every variable the solver has touched. The result is
// ordered by variable id (the Rust crate returns an unspecified order).
func (s *Solver) FetchChanges() []Change {
	if s.shouldClearChanges {
		clear(s.changed)
		s.shouldClearChanges = false
	} else {
		s.shouldClearChanges = true
	}
	s.publicChanges = s.publicChanges[:0]
	vars := make([]Variable, 0, len(s.changed))
	for v := range s.changed {
		vars = append(vars, v)
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].id < vars[j].id })
	for _, v := range vars {
		vd, ok := s.varData[v]
		if !ok {
			continue
		}
		newValue := 0.0
		if r, ok := s.rows.get(vd.symbol); ok {
			newValue = r.constant
		}
		if vd.value != newValue { // NaN != x, as in Rust
			s.publicChanges = append(s.publicChanges, Change{v, newValue})
			vd.value = newValue
		}
	}
	return s.publicChanges
}

// Reset resets the solver to the empty starting condition.
func (s *Solver) Reset() {
	s.rows.clear()
	s.cns = map[*Constraint]tag{}
	s.varData = map[Variable]*varData{}
	s.varForSymbol = map[symbol]Variable{}
	clear(s.changed)
	s.shouldClearChanges = false
	s.edits = map[Variable]*editInfo{}
	s.infeasibleRows = s.infeasibleRows[:0]
	s.objective = newRow(0)
	s.artificial = nil
	s.idTick = 1
}

// getVarSymbol returns the symbol for v, creating it if needed.
func (s *Solver) getVarSymbol(v Variable) symbol {
	vd, ok := s.varData[v]
	if !ok {
		sym := symbol{s.idTick, symExternal}
		s.varForSymbol[sym] = v
		s.idTick++
		vd = &varData{value: math.NaN(), symbol: sym}
		s.varData[v] = vd
	}
	vd.count++
	return vd.symbol
}

func (s *Solver) createRow(c *Constraint) (*row, tag) {
	expr := c.expr
	r := newRow(expr.Constant)
	for _, term := range expr.Terms {
		if !nearZero(term.Coefficient) {
			sym := s.getVarSymbol(term.Variable)
			if other, ok := s.rows.get(sym); ok {
				r.insertRow(other, term.Coefficient)
			} else {
				r.insertSymbol(sym, term.Coefficient)
			}
		}
	}

	objective := s.objective
	var t tag
	switch c.op {
	case GreaterOrEqual, LessOrEqual:
		coeff := -1.0
		if c.op == LessOrEqual {
			coeff = 1.0
		}
		slack := symbol{s.idTick, symSlack}
		s.idTick++
		r.insertSymbol(slack, coeff)
		if c.strength < Required {
			errSym := symbol{s.idTick, symError}
			s.idTick++
			r.insertSymbol(errSym, -coeff)
			objective.insertSymbol(errSym, c.strength)
			t = tag{marker: slack, other: errSym}
		} else {
			t = tag{marker: slack, other: invalidSymbol()}
		}
	case Equal:
		if c.strength < Required {
			errplus := symbol{s.idTick, symError}
			s.idTick++
			errminus := symbol{s.idTick, symError}
			s.idTick++
			r.insertSymbol(errplus, -1.0)
			r.insertSymbol(errminus, 1.0)
			objective.insertSymbol(errplus, c.strength)
			objective.insertSymbol(errminus, c.strength)
			t = tag{marker: errplus, other: errminus}
		} else {
			dummy := symbol{s.idTick, symDummy}
			s.idTick++
			r.insertSymbol(dummy, 1.0)
			t = tag{marker: dummy, other: invalidSymbol()}
		}
	}

	if r.constant < 0 {
		r.reverseSign()
	}
	return r, t
}

func chooseSubject(r *row, t tag) symbol {
	for _, c := range r.cells {
		if c.sym.typ == symExternal {
			return c.sym
		}
	}
	if t.marker.typ == symSlack || t.marker.typ == symError {
		if r.coefficientFor(t.marker) < 0 {
			return t.marker
		}
	}
	if t.other.typ == symSlack || t.other.typ == symError {
		if r.coefficientFor(t.other) < 0 {
			return t.other
		}
	}
	return invalidSymbol()
}

func (s *Solver) addWithArtificialVariable(r *row) (bool, error) {
	art := symbol{s.idTick, symSlack}
	s.idTick++
	s.rows.insert(art, r.clone())
	s.artificial = r.clone()

	artificial := s.artificial
	if err := s.optimise(artificial); err != nil {
		return false, err
	}
	success := nearZero(artificial.constant)
	s.artificial = nil

	if ar, ok := s.rows.remove(art); ok {
		if len(ar.cells) == 0 {
			return success, nil
		}
		entering := anyPivotableSymbol(ar)
		if entering.typ == symInvalid {
			return false, nil
		}
		ar.solveForSymbols(art, entering)
		s.substitute(entering, ar)
		s.rows.insert(entering, ar)
	}

	for _, sym := range s.rows.keys {
		s.rows.m[sym].remove(art)
	}
	s.objective.remove(art)
	return success, nil
}

func (s *Solver) substitute(sym symbol, r *row) {
	for _, other := range s.rows.keys {
		otherRow := s.rows.m[other]
		constantChanged := otherRow.substitute(sym, r)
		if other.typ == symExternal && constantChanged {
			s.varChanged(s.varForSymbol[other])
		}
		if other.typ != symExternal && otherRow.constant < 0 {
			s.infeasibleRows = append(s.infeasibleRows, other)
		}
	}
	s.objective.substitute(sym, r)
	if s.artificial != nil {
		s.artificial.substitute(sym, r)
	}
}

func (s *Solver) optimise(objective *row) error {
	for {
		entering := getEnteringSymbol(objective)
		if entering.typ == symInvalid {
			return nil
		}
		leaving, r, ok := s.getLeavingRow(entering)
		if !ok {
			return InternalSolverError("The objective is unbounded")
		}
		r.solveForSymbols(leaving, entering)
		s.substitute(entering, r)
		if entering.typ == symExternal && r.constant != 0 {
			s.varChanged(s.varForSymbol[entering])
		}
		s.rows.insert(entering, r)
	}
}

func (s *Solver) dualOptimise() error {
	for len(s.infeasibleRows) > 0 {
		leaving := s.infeasibleRows[len(s.infeasibleRows)-1]
		s.infeasibleRows = s.infeasibleRows[:len(s.infeasibleRows)-1]

		r, ok := s.rows.get(leaving)
		if !ok || r.constant >= 0 {
			continue
		}
		s.rows.remove(leaving)
		entering := s.getDualEnteringSymbol(r)
		if entering.typ == symInvalid {
			return InternalSolverError("Dual optimise failed.")
		}
		r.solveForSymbols(leaving, entering)
		s.substitute(entering, r)
		if entering.typ == symExternal && r.constant != 0 {
			s.varChanged(s.varForSymbol[entering])
		}
		s.rows.insert(entering, r)
	}
	return nil
}

func getEnteringSymbol(objective *row) symbol {
	for _, c := range objective.cells {
		if c.sym.typ != symDummy && c.val < 0 {
			return c.sym
		}
	}
	return invalidSymbol()
}

func (s *Solver) getDualEnteringSymbol(r *row) symbol {
	entering := invalidSymbol()
	ratio := math.Inf(1)
	for _, c := range r.cells {
		if c.val > 0 && c.sym.typ != symDummy {
			coeff := s.objective.coefficientFor(c.sym)
			rr := coeff / c.val
			if rr < ratio {
				ratio = rr
				entering = c.sym
			}
		}
	}
	return entering
}

func anyPivotableSymbol(r *row) symbol {
	for _, c := range r.cells {
		if c.sym.typ == symSlack || c.sym.typ == symError {
			return c.sym
		}
	}
	return invalidSymbol()
}

func (s *Solver) getLeavingRow(entering symbol) (symbol, *row, bool) {
	ratio := math.Inf(1)
	var found symbol
	ok := false
	for _, sym := range s.rows.keys {
		if sym.typ != symExternal {
			r := s.rows.m[sym]
			temp := r.coefficientFor(entering)
			if temp < 0 {
				tempRatio := -r.constant / temp
				if tempRatio < ratio {
					ratio = tempRatio
					found = sym
					ok = true
				}
			}
		}
	}
	if !ok {
		return symbol{}, nil, false
	}
	r, _ := s.rows.remove(found)
	return found, r, true
}

func (s *Solver) getMarkerLeavingRow(marker symbol) (symbol, *row, bool) {
	r1 := math.Inf(1)
	r2 := r1
	var first, second, third symbol
	hasFirst, hasSecond, hasThird := false, false, false
	for _, sym := range s.rows.keys {
		r := s.rows.m[sym]
		c := r.coefficientFor(marker)
		if c == 0 {
			continue
		}
		if sym.typ == symExternal {
			third, hasThird = sym, true
		} else if c < 0 {
			rr := -r.constant / c
			if rr < r1 {
				r1 = rr
				first, hasFirst = sym, true
			}
		} else {
			rr := r.constant / c
			if rr < r2 {
				r2 = rr
				second, hasSecond = sym, true
			}
		}
	}
	var chosen symbol
	switch {
	case hasFirst:
		chosen = first
	case hasSecond:
		chosen = second
	case hasThird:
		chosen = third
	default:
		return symbol{}, nil, false
	}
	if chosen.typ == symExternal && s.rows.m[chosen].constant != 0 {
		s.varChanged(s.varForSymbol[chosen])
	}
	r, ok := s.rows.remove(chosen)
	return chosen, r, ok
}

func (s *Solver) removeConstraintEffects(c *Constraint, t tag) {
	if t.marker.typ == symError {
		s.removeMarkerEffects(t.marker, c.strength)
	} else if t.other.typ == symError {
		s.removeMarkerEffects(t.other, c.strength)
	}
}

func (s *Solver) removeMarkerEffects(marker symbol, strength float64) {
	if r, ok := s.rows.get(marker); ok {
		s.objective.insertRow(r, -strength)
	} else {
		s.objective.insertSymbol(marker, -strength)
	}
}

func allDummies(r *row) bool {
	for _, c := range r.cells {
		if c.sym.typ != symDummy {
			return false
		}
	}
	return true
}

// GetValue returns the current value of a variable (0 if unknown).
func (s *Solver) GetValue(v Variable) float64 {
	if vd, ok := s.varData[v]; ok {
		if r, ok := s.rows.get(vd.symbol); ok {
			return r.constant
		}
	}
	return 0
}

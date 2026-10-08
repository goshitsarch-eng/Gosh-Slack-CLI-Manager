// Package cassowary is a port of the Rust cassowary crate (version 0.3.0), an
// implementation of the Cassowary incremental linear constraint solving
// algorithm (Badros et al., 2001). It exists so that the TUI layout engine can
// reproduce ratatui's layout results exactly.
//
// The Rust crate stores its tableau in std HashMaps, whose iteration order is
// randomised per process. Wherever the Rust code picks "the first" symbol or
// row while iterating such a map, this port iterates in ascending symbol id
// order instead (symbol ids are allocated sequentially, so this is also
// creation order). This makes the solver deterministic; for problems with a
// unique optimum the result is identical to the Rust crate's.
package cassowary

import (
	"fmt"
	"math"
	"sync/atomic"
)

var variableID atomic.Uint64

// Variable identifies a variable for the constraint solver. Each variable
// returned by NewVariable is unique; copies of a Variable refer to the same
// variable.
type Variable struct {
	id uint64
}

// NewVariable produces a new unique variable for use in constraint solving.
func NewVariable() Variable {
	return Variable{id: variableID.Add(1) - 1}
}

// ID returns the variable's unique identifier.
func (v Variable) ID() uint64 { return v.id }

// Term is a variable and a coefficient to multiply that variable by.
type Term struct {
	Variable    Variable
	Coefficient float64
}

// NewTerm constructs a Term.
func NewTerm(v Variable, coefficient float64) Term {
	return Term{Variable: v, Coefficient: coefficient}
}

// Neg returns the term multiplied by minus one.
func (t Term) Neg() Term { return Term{t.Variable, -t.Coefficient} }

// Mul returns the term with its coefficient multiplied by f.
func (t Term) Mul(f float64) Term { return Term{t.Variable, t.Coefficient * f} }

// Div returns the term with its coefficient divided by f.
func (t Term) Div(f float64) Term { return Term{t.Variable, t.Coefficient / f} }

// Expr converts the term to an expression.
func (t Term) Expr() Expression { return Expression{Terms: []Term{t}} }

// Expression is a linear combination of variables plus a constant.
type Expression struct {
	Terms    []Term
	Constant float64
}

// NewExpression constructs an expression from terms and a constant.
func NewExpression(terms []Term, constant float64) Expression {
	return Expression{Terms: append([]Term(nil), terms...), Constant: constant}
}

// Const constructs an expression consisting only of a constant.
func Const(v float64) Expression { return Expression{Constant: v} }

// Expr converts the variable to the expression `1 * v`.
func (v Variable) Expr() Expression { return Expression{Terms: []Term{{v, 1}}} }

// Mul returns the term `f * v`.
func (v Variable) Mul(f float64) Term { return Term{v, f} }

// Sub returns the expression `v - o` (Rust: Variable - Variable).
func (v Variable) Sub(o Variable) Expression {
	return Expression{Terms: []Term{{v, 1}, {o, -1}}}
}

// Add returns the expression `v + o` (Rust: Variable + Variable).
func (v Variable) Add(o Variable) Expression {
	return Expression{Terms: []Term{{v, 1}, {o, 1}}}
}

func (e Expression) clone() Expression {
	return Expression{Terms: append([]Term(nil), e.Terms...), Constant: e.Constant}
}

// Negate returns the expression multiplied by minus one.
func (e Expression) Negate() Expression {
	r := e.clone()
	r.Constant = -r.Constant
	for i := range r.Terms {
		r.Terms[i] = r.Terms[i].Neg()
	}
	return r
}

// Add returns `e + o`; the terms of o are appended after those of e.
func (e Expression) Add(o Expression) Expression {
	r := e.clone()
	r.Terms = append(r.Terms, o.Terms...)
	r.Constant += o.Constant
	return r
}

// Sub returns `e - o`; the negated terms of o are appended after those of e.
func (e Expression) Sub(o Expression) Expression {
	n := o.Negate()
	r := e.clone()
	r.Terms = append(r.Terms, n.Terms...)
	r.Constant += n.Constant
	return r
}

// AddTerm returns e with t appended.
func (e Expression) AddTerm(t Term) Expression {
	r := e.clone()
	r.Terms = append(r.Terms, t)
	return r
}

// SubVar returns e with the term `-1 * v` appended.
func (e Expression) SubVar(v Variable) Expression {
	return e.AddTerm(Term{v, -1})
}

// AddConst returns `e + c`.
func (e Expression) AddConst(c float64) Expression {
	r := e.clone()
	r.Constant += c
	return r
}

// SubConst returns `e - c`.
func (e Expression) SubConst(c float64) Expression {
	r := e.clone()
	r.Constant -= c
	return r
}

// Mul returns `e * f`.
func (e Expression) Mul(f float64) Expression {
	r := e.clone()
	r.Constant *= f
	for i := range r.Terms {
		r.Terms[i] = r.Terms[i].Mul(f)
	}
	return r
}

// Div returns `e / f`.
func (e Expression) Div(f float64) Expression {
	r := e.clone()
	r.Constant /= f
	for i := range r.Terms {
		r.Terms[i] = r.Terms[i].Div(f)
	}
	return r
}

// Strengths. Strengths are real numbers; the strongest legal strength is
// Required and the weakest is 0.
const (
	Required = 1_001_001_000.0
	Strong   = 1_000_000.0
	Medium   = 1_000.0
	Weak     = 1.0
)

// rustMax mirrors f64::max (NaN operands are ignored).
func rustMax(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if a > b {
		return a
	}
	return b
}

// rustMin mirrors f64::min (NaN operands are ignored).
func rustMin(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// CreateStrength creates a strength as a linear combination of Strong, Medium
// and Weak strengths, corresponding to a, b and c respectively, each
// multiplied by w (Rust: strength::create).
func CreateStrength(a, b, c, w float64) float64 {
	return rustMin(rustMax(a*w, 0), 1000)*1_000_000.0 +
		rustMin(rustMax(b*w, 0), 1000)*1000.0 +
		rustMin(rustMax(c*w, 0), 1000)
}

// ClipStrength clips a strength value to the legal range (Rust: strength::clip).
func ClipStrength(s float64) float64 {
	return rustMax(rustMin(s, Required), 0)
}

// RelationalOperator is the relation a constraint specifies.
type RelationalOperator uint8

const (
	// LessOrEqual is `<=`.
	LessOrEqual RelationalOperator = iota
	// Equal is `==`.
	Equal
	// GreaterOrEqual is `>=`.
	GreaterOrEqual
)

func (o RelationalOperator) String() string {
	switch o {
	case LessOrEqual:
		return "<="
	case Equal:
		return "=="
	default:
		return ">="
	}
}

// Constraint is the equation `expr op 0` with an associated strength.
// Constraints are compared by identity (pointer), like the Rust crate's
// Arc-based Constraint.
type Constraint struct {
	expr     Expression
	op       RelationalOperator
	strength float64
}

// NewConstraint constructs the constraint `e op 0` with the given strength.
func NewConstraint(e Expression, op RelationalOperator, strength float64) *Constraint {
	return &Constraint{expr: e.clone(), op: op, strength: strength}
}

// Eq builds `lhs == rhs` (Rust: lhs |EQ(strength)| rhs).
func Eq(lhs Expression, strength float64, rhs Expression) *Constraint {
	return NewConstraint(lhs.Sub(rhs), Equal, strength)
}

// Le builds `lhs <= rhs` (Rust: lhs |LE(strength)| rhs).
func Le(lhs Expression, strength float64, rhs Expression) *Constraint {
	return NewConstraint(lhs.Sub(rhs), LessOrEqual, strength)
}

// Ge builds `lhs >= rhs` (Rust: lhs |GE(strength)| rhs).
func Ge(lhs Expression, strength float64, rhs Expression) *Constraint {
	return NewConstraint(lhs.Sub(rhs), GreaterOrEqual, strength)
}

// Expr returns the left hand side of the constraint equation.
func (c *Constraint) Expr() Expression { return c.expr }

// Op returns the relational operator.
func (c *Constraint) Op() RelationalOperator { return c.op }

// Strength returns the strength of the constraint.
func (c *Constraint) Strength() float64 { return c.strength }

// Errors returned by the solver.
var (
	ErrDuplicateConstraint     = fmt.Errorf("cassowary: duplicate constraint")
	ErrUnsatisfiableConstraint = fmt.Errorf("cassowary: unsatisfiable constraint")
	ErrUnknownConstraint       = fmt.Errorf("cassowary: unknown constraint")
	ErrDuplicateEditVariable   = fmt.Errorf("cassowary: duplicate edit variable")
	ErrBadRequiredStrength     = fmt.Errorf("cassowary: edit variable strength cannot be REQUIRED")
	ErrUnknownEditVariable     = fmt.Errorf("cassowary: unknown edit variable")
)

// InternalSolverError reports that the solver entered an invalid state.
type InternalSolverError string

func (e InternalSolverError) Error() string { return "cassowary: internal solver error: " + string(e) }

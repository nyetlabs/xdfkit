package xdf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.nyet.org/xdfkit/model"
)

// lin is k*X + c, or k/X + c when recip.
type lin struct {
	k, c  float64
	recip bool
}

func (a lin) constant() bool { return a.k == 0 }

// parseEquation reads a TunerPro equation in X as a conversion. It accepts
// numbers, X, + - * / and parentheses, as long as the result is factor*X +
// offset or factor/X + offset (KP's reciprocal form). Empty means X.
func parseEquation(s string) (model.Conversion, error) {
	if strings.TrimSpace(s) == "" {
		return model.Conversion{Factor: 1}, nil
	}
	p := &eqParser{s: s}
	v, err := p.expr()
	if err == nil && p.peek() != 0 {
		err = fmt.Errorf("unexpected %q", p.s[p.i:])
	}
	if err != nil {
		return model.Conversion{}, err
	}
	if v.constant() {
		return model.Conversion{}, errors.New("no X")
	}
	// adding 0 turns -0 into 0, so "-2 * X" has offset 0, not -0
	return model.Conversion{Factor: v.k, Offset: v.c + 0, Reciprocal: v.recip}, nil
}

type eqParser struct {
	s string
	i int
}

// peek returns the next non-space byte, 0 at the end.
func (p *eqParser) peek() byte {
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
	if p.i == len(p.s) {
		return 0
	}
	return p.s[p.i]
}

func (p *eqParser) expr() (lin, error) {
	a, err := p.term()
	for err == nil {
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.i++
		var b lin
		if b, err = p.term(); err != nil {
			break
		}
		if op == '-' {
			b = lin{-b.k, -b.c, b.recip}
		}
		if !a.constant() && !b.constant() && a.recip != b.recip {
			return a, errors.New("mixes X and 1/X")
		}
		a = lin{a.k + b.k, a.c + b.c, a.recip || b.recip}
	}
	return a, err
}

func (p *eqParser) term() (lin, error) {
	a, err := p.unary()
	for err == nil {
		op := p.peek()
		if op != '*' && op != '/' {
			break
		}
		p.i++
		var b lin
		if b, err = p.unary(); err != nil {
			break
		}
		switch {
		case op == '*' && a.constant():
			a = lin{a.c * b.k, a.c * b.c, b.recip}
		case op == '*' && b.constant():
			a = lin{a.k * b.c, a.c * b.c, a.recip}
		case op == '/' && b.constant():
			a = lin{a.k / b.c, a.c / b.c, a.recip}
		case op == '/' && a.constant() && b.c == 0 && !b.recip:
			a = lin{a.c / b.k, 0, true}
		default:
			return a, errors.New("not linear in X")
		}
	}
	return a, err
}

func (p *eqParser) unary() (lin, error) {
	switch p.peek() {
	case '-':
		p.i++
		a, err := p.unary()
		return lin{-a.k, -a.c, a.recip}, err
	case '+':
		p.i++
		return p.unary()
	case '(':
		p.i++
		a, err := p.expr()
		if err == nil && p.peek() != ')' {
			err = errors.New("missing )")
		}
		p.i++
		return a, err
	case 'X', 'x':
		p.i++
		return lin{k: 1}, nil
	case 0:
		return lin{}, errors.New("unexpected end")
	}
	j := p.i
	for j < len(p.s) && (strings.IndexByte("0123456789.eE", p.s[j]) >= 0 ||
		(p.s[j] == '+' || p.s[j] == '-') && j > p.i && (p.s[j-1] == 'e' || p.s[j-1] == 'E')) {
		j++
	}
	f, err := strconv.ParseFloat(p.s[p.i:j], 64)
	if err != nil || j == p.i {
		return lin{}, fmt.Errorf("unexpected %q", p.s[p.i:])
	}
	p.i = j
	return lin{c: f}, nil
}

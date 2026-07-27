package harness

import (
	"fmt"
	"strings"
	"unicode"
)

// MarkerExpr is a compiled pytest-style marker expression.
//
// The grammar matches what `pytest -m` accepts, which is what makes an
// upstream invocation like
//
//	-m 'bucket_logging and not fails_without_logging_rollover'
//
// work here unchanged:
//
//	expr := or
//	or   := and ('or' and)*
//	and  := not ('and' not)*
//	not  := 'not' not | atom
//	atom := '(' expr ')' | IDENT
//
// An empty expression matches everything.
type MarkerExpr struct {
	node node
}

type node interface {
	eval(markers map[string]bool) bool
}

type identNode struct{ name string }

func (n identNode) eval(m map[string]bool) bool { return m[n.name] }

type notNode struct{ inner node }

func (n notNode) eval(m map[string]bool) bool { return !n.inner.eval(m) }

type andNode struct{ lhs, rhs node }

func (n andNode) eval(m map[string]bool) bool { return n.lhs.eval(m) && n.rhs.eval(m) }

type orNode struct{ lhs, rhs node }

func (n orNode) eval(m map[string]bool) bool { return n.lhs.eval(m) || n.rhs.eval(m) }

// ParseMarkerExpr compiles a marker expression. An empty string yields an
// expression that matches every test.
func ParseMarkerExpr(s string) (MarkerExpr, error) {
	if strings.TrimSpace(s) == "" {
		return MarkerExpr{}, nil
	}
	toks, err := lexMarkers(s)
	if err != nil {
		return MarkerExpr{}, err
	}
	p := &markerParser{toks: toks}
	n, err := p.parseOr()
	if err != nil {
		return MarkerExpr{}, err
	}
	if p.pos != len(p.toks) {
		return MarkerExpr{}, fmt.Errorf("unexpected %q", p.toks[p.pos])
	}
	return MarkerExpr{node: n}, nil
}

// Match reports whether a test carrying the given markers is selected.
func (e MarkerExpr) Match(markers []string) bool {
	if e.node == nil {
		return true
	}
	set := make(map[string]bool, len(markers))
	for _, m := range markers {
		set[m] = true
	}
	return e.node.eval(set)
}

// lexMarkers splits an expression into identifiers and parentheses. Marker
// names are Python identifiers, so anything alphanumeric or underscore is part
// of a name and everything else must be a paren or whitespace.
func lexMarkers(s string) ([]string, error) {
	var toks []string
	for i := 0; i < len(s); {
		c := rune(s[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		case isIdentRune(c):
			j := i
			for j < len(s) && isIdentRune(rune(s[j])) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q in marker expression", c)
		}
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty marker expression")
	}
	return toks, nil
}

func isIdentRune(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

type markerParser struct {
	toks []string
	pos  int
}

func (p *markerParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *markerParser) parseOr() (node, error) {
	lhs, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "or" {
		p.pos++
		rhs, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		lhs = orNode{lhs, rhs}
	}
	return lhs, nil
}

func (p *markerParser) parseAnd() (node, error) {
	lhs, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.peek() == "and" {
		p.pos++
		rhs, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		lhs = andNode{lhs, rhs}
	}
	return lhs, nil
}

func (p *markerParser) parseNot() (node, error) {
	if p.peek() == "not" {
		p.pos++
		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return notNode{inner}, nil
	}
	return p.parseAtom()
}

func (p *markerParser) parseAtom() (node, error) {
	tok := p.peek()
	switch tok {
	case "":
		return nil, fmt.Errorf("unexpected end of marker expression")
	case "(":
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return inner, nil
	case ")":
		return nil, fmt.Errorf("unexpected closing parenthesis")
	case "and", "or":
		return nil, fmt.Errorf("unexpected operator %q", tok)
	default:
		p.pos++
		return identNode{tok}, nil
	}
}

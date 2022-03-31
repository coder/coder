package authztest

import (
	"fmt"
)

type Parser struct {
	input string
	stack []interface{}
	grp   SetGroup

	setI []Iterable
}

func ParseRole(grp SetGroup, input string) *Role {
	p := NewParser(grp, input)
	p.parse()
	return NewRole(p.setI...)
}

func Parse(grp SetGroup, input string) []Iterable {
	p := NewParser(grp, input)
	p.parse()
	return p.setI
}

func NewParser(grp SetGroup, input string) *Parser {

	return &Parser{
		grp:   grp,
		input: input,
		stack: make([]interface{}, 0),
	}
}

func (p *Parser) skipSpace(ptr int) int {
	for ptr < len(p.input) {
		r := p.input[ptr]
		switch r {
		case ' ', '\t', '\n':
			ptr++
		default:
			return ptr
		}
	}
	return ptr
}

func (p *Parser) parse() {
	ptr := 0
	for ptr < len(p.input) {
		ptr = p.skipSpace(ptr)
		r := p.input[ptr]
		switch r {
		case ' ':
			ptr++
		case 'w', 's', 'o', 'm', 'u':
			// Time to look ahead for the grp
			ptr++
			ptr = p.handleLevel(r, ptr)
		default:
			panic(fmt.Errorf("cannot handle '%c' at %d", r, ptr))
		}
	}
}

func (p *Parser) handleLevel(l uint8, ptr int) int {
	var lg LevelGroup
	switch l {
	case 'w':
		lg = p.grp.Wildcard()
	case 's':
		lg = p.grp.Site()
	case 'o':
		lg = p.grp.AllOrgs()
	case 'm':
		lg = p.grp.OrgMem()
	case 'u':
		lg = p.grp.User()
	}

	// time to look ahead. Find the parenthesis
	var sets []Set
	var start bool
	var stop bool
	for {
		ptr = p.skipSpace(ptr)
		r := p.input[ptr]
		if r != '(' && !start {
			panic(fmt.Sprintf("Expect a parenthesis at %d", ptr))
		}
		switch r {
		case '(':
			start = true
		case ')':
			stop = true
		case 'p':
			sets = append(sets, lg.Positive())
		case 'n':
			sets = append(sets, lg.Negative())
		case 'a':
			sets = append(sets, lg.Abstain())
		case '*':
			sets = append(sets, lg.All())
		case 'e':
			// Add the empty perm
			sets = append(sets, Set{nil})
		default:
			panic(fmt.Errorf("unsupported '%c' for level set", r))
		}
		ptr++
		if stop {
			p.setI = append(p.setI, Union(sets...))
			return ptr
		}
	}
}

//func (p *Parser)

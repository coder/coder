// Package clibase offers an all-in-one solution for a highly configurable CLI
// application. Within Coder, we use it for all of our subcommands, which
// demands more functionality than cobra/viber offers.
//
// The Command interface is loosely based on the chi middleware pattern and
// http.Handler/HandlerFunc.
package clibase

import (
	"strings"

	"golang.org/x/exp/maps"
)

// Group describes a hierarchy of groups that an option or command belongs to.
type Group struct {
	Parent      *Group  `json:"parent,omitempty"`
	Name        string  `json:"name,omitempty"`
	Children    []Group `json:"children,omitempty"`
	Description string  `json:"description,omitempty"`
}

func (g *Group) AddChild(child Group) {
	child.Parent = g
	g.Children = append(g.Children, child)
}

// Ancestry returns the group and all of its parents, in order.
func (g *Group) Ancestry() []Group {
	if g == nil {
		return nil
	}

	groups := []Group{*g}
	for p := g.Parent; p != nil; p = p.Parent {
		// Prepend to the slice so that the order is correct.
		groups = append([]Group{*p}, groups...)
	}
	return groups
}

func (g *Group) FullName() string {
	var names []string
	for _, g := range g.Ancestry() {
		names = append(names, g.Name)
	}
	return strings.Join(names, " / ")
}

// Annotations is an arbitrary key-mapping used to extend the Option and Command types.
// Its methods won't panic if the map is nil.
type Annotations map[string]string

// Mark sets a value on the annotations map, creating one
// if it doesn't exist. Mark does not mutate the original and
// returns a copy. It is suitable for chaining.
func (a Annotations) Mark(key string, value string) Annotations {
	var aa Annotations
	if a != nil {
		aa = maps.Clone(a)
	} else {
		aa = make(Annotations)
	}
	aa[key] = value
	return aa
}

// IsSet returns true if the key is set in the annotations map.
func (a Annotations) IsSet(key string) bool {
	if a == nil {
		return false
	}
	_, ok := a[key]
	return ok
}

// Get retrieves a key from the map, returning false if the key is not found
// or the map is nil.
func (a Annotations) Get(key string) (string, bool) {
	if a == nil {
		return "", false
	}
	v, ok := a[key]
	return v, ok
}

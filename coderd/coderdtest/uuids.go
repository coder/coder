package coderdtest

import "github.com/google/uuid"

type DeterministicUUIDGenerator struct {
	Named map[string]uuid.UUID
}

func NewDeterministicUUIDGenerator() *DeterministicUUIDGenerator {
	return &DeterministicUUIDGenerator{
		Named: make(map[string]uuid.UUID),
	}
}

func (d *DeterministicUUIDGenerator) ID(name string) uuid.UUID {
	if v, ok := d.Named[name]; ok {
		return v
	}
	d.Named[name] = uuid.New()
	return d.Named[name]
}

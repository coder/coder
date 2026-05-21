package coderdtest

import "github.com/google/uuid"

// DeterministicUUIDGenerator allows "naming" uuids for unit tests.
// An example of where this is useful, is when a tabled test references
// a UUID that is not yet known. An alternative to this would be to
// hard code some UUID strings, but these strings are not human friendly.
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

package authztest

import (
	"testing"
)

func TestPermissionSet(t *testing.T) {
	all := AllPermissions()
	c := NewCache(all)

	c.Site().Positive()

}

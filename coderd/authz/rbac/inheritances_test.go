package rbac

import (
	"reflect"
	"testing"
)

const (
	Admin   Role = "admin"
	Member  Role = "member"
	Auditor Role = "auditor"
)

var testInheritances = Inheritances{
	Admin: Roles{Member, Auditor},
}

func Test_Inheritances_GetAncestors(t *testing.T) {
	got := testInheritances.GetAncestors(Admin)
	want := Roles{Member, Auditor}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got: %v; want: %v", got, want)
	}
}

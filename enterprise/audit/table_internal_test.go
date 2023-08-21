package audit

import (
	"fmt"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// TestAuditableResources ensures that all auditable resources are included in
// the Auditable interface and vice versa.
//
//nolint:tparallel
func TestAuditableResources(t *testing.T) {
	t.Parallel()

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypes | packages.NeedDeps,
	}, "../../coderd/audit")
	require.NoError(t, err)

	if len(pkgs) != 1 {
		t.Fatal("expected one package")
	}
	auditPkg := pkgs[0]
	auditableType := auditPkg.Types.Scope().Lookup("Auditable")

	// If any of these type cast fails, our Auditable interface is not what we
	// expect it to be.
	named, ok := auditableType.(*types.TypeName)
	require.True(t, ok, "expected Auditable to be a type name")

	interfaceType, ok := named.Type().(*types.Named).Underlying().(*types.Interface)
	require.True(t, ok, "expected Auditable to be an interface")

	unionType, ok := interfaceType.EmbeddedType(0).(*types.Union)
	require.True(t, ok, "expected Auditable to be a union")

	found := make(map[string]bool)
	expectedList := make([]string, 0)
	// Now we check we have all the resources in the AuditableResources
	for i := 0; i < unionType.Len(); i++ {
		// All types come across like 'github.com/coder/coder/v2/coderd/database.<type>'
		typeName := unionType.Term(i).Type().String()
		_, ok := AuditableResources[typeName]
		assert.True(t, ok, "missing resource %q from AuditableResources", typeName)
		found[typeName] = true
		expectedList = append(expectedList, typeName)
	}

	// Also check that all resources in the table are in the union. We could
	// have extra resources here.
	for name := range AuditableResources {
		_, ok := found[name]
		assert.True(t, ok, "extra resource %q found in AuditableResources", name)
	}

	// Various functions that have switch statements to include all Auditable
	// resources. Make sure we have all types supported.
	// nolint:paralleltest
	t.Run("ResourceID", func(t *testing.T) {
		// The function being tested, provided here to make it easier to find
		_ = audit.ResourceID[database.APIKey]
		testAuditFunctionWithSwitch(t, auditPkg, "ResourceID", expectedList)
	})

	// nolint:paralleltest
	t.Run("ResourceType", func(t *testing.T) {
		// The function being tested, provided here to make it easier to find
		_ = audit.ResourceType[database.APIKey]
		testAuditFunctionWithSwitch(t, auditPkg, "ResourceType", expectedList)
	})

	// nolint:paralleltest
	t.Run("ResourceTarget", func(t *testing.T) {
		// The function being tested, provided here to make it easier to find
		_ = audit.ResourceTarget[database.APIKey]
		testAuditFunctionWithSwitch(t, auditPkg, "ResourceTarget", expectedList)
	})
}

// testAuditFunctionWithSwitch is a helper function to test that a function has
// a typed switch statement that includes all the types in expectedTypes.
func testAuditFunctionWithSwitch(t *testing.T, pkg *packages.Package, funcName string, expectedTypes []string) {
	t.Helper()

	f, ok := pkg.Types.Scope().Lookup(funcName).(*types.Func)
	require.True(t, ok, fmt.Sprintf("expected %s to be a function", funcName))
	switchCases := findSwitchTypes(f)
	for _, expected := range expectedTypes {
		if !slice.Contains(switchCases, expected) {
			t.Errorf("%s switch statement is missing type %q. Include it in the switch case block", funcName, expected)
		}
	}
	for _, sc := range switchCases {
		if !slice.Contains(expectedTypes, sc) {
			t.Errorf("%s switch statement has unexpected type %q. Remove it from the switch case block", funcName, sc)
		}
	}
}

// findSwitchTypes is a helper function to find all types a switch statement in
// the function body of f has.
func findSwitchTypes(f *types.Func) []string {
	caseTypes := make([]string, 0)
	switches := returnSwitchBlocks(f.Scope())
	for _, sc := range switches {
		scTypes := findCaseTypes(sc)
		caseTypes = append(caseTypes, scTypes...)
	}
	return caseTypes
}

func returnSwitchBlocks(sc *types.Scope) []*types.Scope {
	switches := make([]*types.Scope, 0)
	for i := 0; i < sc.NumChildren(); i++ {
		child := sc.Child(i)
		cStr := child.String()
		// This is the easiest way to tell if it is a switch statement.
		if strings.Contains(cStr, "type switch scope") {
			switches = append(switches, child)
		}
	}
	return switches
}

// findCaseTypes returns all case types in a typed switch statement. Excluding
// the "Default:" case.
func findCaseTypes(sc *types.Scope) []string {
	caseTypes := make([]string, 0)
	for i := 0; i < sc.NumChildren(); i++ {
		child := sc.Child(i)
		for _, name := range child.Names() {
			obj := child.Lookup(name).Type()
			typeName := obj.String()
			// Ignore the "Default:" case
			if typeName != "any" {
				caseTypes = append(caseTypes, typeName)
			}
		}
	}
	return caseTypes
}

package audit

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// TestAuditableResources ensures that all auditable resources are included in
// the Auditable interface and vice versa.
func TestAuditableResources(t *testing.T) {
	t.Parallel()

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypes,
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
	// Now we check we have all the resources in the AuditableResources
	for i := 0; i < unionType.Len(); i++ {
		// All types come across like 'github.com/coder/coder/coderd/database.<type>'
		typeName := unionType.Term(i).Type().String()
		_, ok := AuditableResources[typeName]
		assert.True(t, ok, "missing resource %q from AuditableResources", typeName)
		found[typeName] = true
	}

	// Also check that all resources in the table are in the union. We could
	// have extra resources here.
	for name := range AuditableResources {
		_, ok := found[name]
		assert.True(t, ok, "extra resource %q found in AuditableResources", name)
	}
}

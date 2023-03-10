package audit

import (
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// TestAuditableResources ensures that all auditable resources are included in
// the Auditable interface and vice versa.
func TestAuditableResources(t *testing.T) {
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

	found := 0
	// Now we check we have all the resources in the AuditableResources
	for i := 0; i < unionType.Len(); i++ {
		// All types come across like 'github.com/coder/coder/coderd/database.<type>'
		typeName := unionType.Term(i).Type().String()
		strings.TrimPrefix(typeName, "github.com/coder/coder/coderd/database.")
		_, ok := AuditableResources[typeName]
		require.True(t, ok, "missing resource %q from AuditableResources", typeName)
		found++
	}

	// It will not be possible to have extra resources in AuditableResources.
	// But it can't hurt to check.
	if found > len(AuditableResources) {
		t.Errorf("extra resources found in AuditableResources. Expected %d, got %d", found, len(AuditableResources))
	}
}

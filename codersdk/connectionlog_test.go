package codersdk_test

import (
	"go/constant"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/coder/coder/v2/codersdk"
)

func TestConnectionTypeOfApp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		appName string
		want    codersdk.ConnectionType
	}{
		{"App", "cursor", codersdk.ConnectionTypeVSCode},
		{"AppNamedLikeItsFamily", "jetbrains", codersdk.ConnectionTypeJetBrains},
		{"Normalized", "Code-Server", codersdk.ConnectionTypeVSCode},
		{"Unregistered", "an_unregistered_ide", codersdk.ConnectionTypeUnknown},
		// Clients pick their own names, so this is just an unregistered app.
		{"WebTypeName", "tunnel", codersdk.ConnectionTypeUnknown},
		{"SFTP", "sftp", codersdk.ConnectionTypeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.ConnectionTypeOfApp(tc.appName))
		})
	}
}

func TestConnectionTypeAppNames(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"jetbrains"}, codersdk.ConnectionTypeJetBrains.AppNames())
	require.Equal(t, []string{"ssh", "zed"}, codersdk.ConnectionTypeSSH.AppNames())
	require.Empty(t, codersdk.ConnectionTypeTunnel.AppNames())
	require.Empty(t, codersdk.ConnectionTypeUnknown.AppNames())
	require.Empty(t, codersdk.ConnectionType("cursor").AppNames())

	// The VS Code family grows, so check membership rather than the list.
	vscode := codersdk.ConnectionTypeVSCode.AppNames()
	require.Contains(t, vscode, "vscode")
	require.Contains(t, vscode, "cursor")
	require.NotContains(t, vscode, "jetbrains")

	known := codersdk.KnownConnectionAppNames()
	require.Contains(t, known, "cursor")
	require.NotContains(t, known, "sftp")
}

func TestConnectionTypeDisplayName(t *testing.T) {
	t.Parallel()

	for typ, want := range map[codersdk.ConnectionType]string{
		codersdk.ConnectionTypeVSCode:          "Visual Studio Code",
		codersdk.ConnectionTypeJetBrains:       "JetBrains",
		codersdk.ConnectionTypeReconnectingPTY: "Web Terminal",
		codersdk.ConnectionTypeWorkspaceApp:    "Workspace App",
		codersdk.ConnectionTypeUnknown:         "Unknown",
	} {
		require.Equal(t, want, typ.DisplayName(), typ)
	}
}

// Each registry family needs a ConnectionType constant with a display name.
func TestConnectionTypeConstantsMatchRegistry(t *testing.T) {
	t.Parallel()

	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes}, ".")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	require.Empty(t, pkgs[0].Errors)

	scope := pkgs[0].Types.Scope()
	connType := scope.Lookup("ConnectionType").Type()
	var declared []codersdk.ConnectionType
	for _, name := range scope.Names() {
		if c, ok := scope.Lookup(name).(*types.Const); ok && types.Identical(c.Type(), connType) {
			declared = append(declared, codersdk.ConnectionType(constant.StringVal(c.Val())))
		}
	}
	require.ElementsMatch(t, declared, codersdk.FilterableConnectionTypes())

	for _, typ := range declared {
		require.NotEqual(t, string(typ), typ.DisplayName(), "ConnectionType %q has no display name", typ)
		if typ != codersdk.ConnectionTypeUnknown && !typ.IsWeb() {
			require.NotEmpty(t, typ.AppNames(), "ConnectionType %q has no app", typ)
		}
	}
}

package agentmcp

import "strings"

const (
	pluginRootPlaceholder = "${PLUGIN_ROOT}"
	pluginDataPlaceholder = "${PLUGIN_DATA}"

	// pluginRootEnv and pluginDataEnv are set on every plugin stdio
	// server and may not be overridden by an entry's env.
	pluginRootEnv = "PLUGIN_ROOT"
	pluginDataEnv = "PLUGIN_DATA"
)

// expandPluginPlaceholders replaces ${PLUGIN_ROOT} and ${PLUGIN_DATA} in
// s with the scope's Root and DataDir. Replacement is a single pass:
// placeholder text inside a substituted value is not expanded again,
// and every other $ sequence is returned verbatim.
func expandPluginPlaceholders(s string, scope PluginScope) string {
	if !strings.Contains(s, "${") {
		return s
	}
	return strings.NewReplacer(
		pluginRootPlaceholder, scope.Root,
		pluginDataPlaceholder, scope.DataDir,
	).Replace(s)
}

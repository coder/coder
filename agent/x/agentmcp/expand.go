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

// expandPluginPlaceholders expands ${PLUGIN_ROOT} and ${PLUGIN_DATA} in
// a single pass, so placeholder text inside a substituted value is not
// expanded again.
func expandPluginPlaceholders(s string, scope PluginScope) string {
	if !strings.Contains(s, "${") {
		return s
	}
	return strings.NewReplacer(
		pluginRootPlaceholder, scope.Root,
		pluginDataPlaceholder, scope.DataDir,
	).Replace(s)
}

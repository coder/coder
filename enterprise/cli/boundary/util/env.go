//nolint:revive,gocritic,errname,unconvert
package util

import "strings"

func MergeEnvs(base []string, extra map[string]string) []string {
	envMap := make(map[string]string)
	for _, env := range base {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for key, value := range extra {
		envMap[key] = value
	}

	merged := make([]string, 0, len(envMap))
	for key, value := range envMap {
		merged = append(merged, key+"="+value)
	}

	return merged
}

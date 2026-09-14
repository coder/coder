//go:build linux || darwin

package terraform_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/testutil"
)

// TestConvertStateGolden compares the output of ConvertState to a golden
// file to prevent regressions. If the logic changes, update the golden files
// accordingly.
//
// This was created to aid in refactoring `ConvertState`.
func TestConvertStateGolden(t *testing.T) {
	t.Parallel()

	testResourceDirectories := filepath.Join("testdata", "resources")
	entries, err := os.ReadDir(testResourceDirectories)
	require.NoError(t, err)

	for _, testDirectory := range entries {
		if !testDirectory.IsDir() {
			continue
		}

		testFiles, err := os.ReadDir(filepath.Join(testResourceDirectories, testDirectory.Name()))
		require.NoError(t, err)

		// ConvertState works on both a plan file and a state file.
		// The test should create a golden file for both.
		for _, step := range []string{"plan", "state"} {
			srcIdc := slices.IndexFunc(testFiles, func(entry os.DirEntry) bool {
				return strings.HasSuffix(entry.Name(), fmt.Sprintf(".tf%s.json", step))
			})
			dotIdx := slices.IndexFunc(testFiles, func(entry os.DirEntry) bool {
				return strings.HasSuffix(entry.Name(), fmt.Sprintf(".tf%s.dot", step))
			})

			// If the directory is missing these files, we cannot run ConvertState
			// on it. So it's skipped.
			if srcIdc == -1 || dotIdx == -1 {
				continue
			}

			t.Run(step+"_"+testDirectory.Name(), func(t *testing.T) {
				t.Parallel()
				testDirectoryPath := filepath.Join(testResourceDirectories, testDirectory.Name())
				planFile := filepath.Join(testDirectoryPath, testFiles[srcIdc].Name())
				dotFile := filepath.Join(testDirectoryPath, testFiles[dotIdx].Name())

				ctx := testutil.Context(t, testutil.WaitMedium)
				logger := slogtest.Make(t, nil)

				// Gather plan
				tfStepRaw, err := os.ReadFile(planFile)
				require.NoError(t, err)

				var modules []*tfjson.StateModule
				switch step {
				case "plan":
					var tfPlan tfjson.Plan
					err = json.Unmarshal(tfStepRaw, &tfPlan)
					require.NoError(t, err)

					modules = []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}
					if tfPlan.PriorState != nil {
						modules = append(modules, tfPlan.PriorState.Values.RootModule)
					}
				case "state":
					var tfState tfjson.State
					err = json.Unmarshal(tfStepRaw, &tfState)
					require.NoError(t, err)
					modules = []*tfjson.StateModule{tfState.Values.RootModule}
				default:
					t.Fatalf("unknown step: %s", step)
				}

				// Gather graph
				dotFileRaw, err := os.ReadFile(dotFile)
				require.NoError(t, err)

				// expectedOutput is `any` to support errors too. If `ConvertState` returns an
				// error, that error is the golden file output.
				var expectedOutput any
				state, err := terraform.ConvertState(ctx, modules, string(dotFileRaw), logger)
				if err == nil {
					sortResources(state.Resources)
					sortExternalAuthProviders(state.ExternalAuthProviders)
					deterministicAppIDs(state.Resources)
					expectedOutput = state
				} else {
					// Write the error to the file then. Track errors as much as valid paths.
					expectedOutput = err.Error()
				}

				expPath := filepath.Join(testDirectoryPath, fmt.Sprintf("converted_state.%s.golden", step))
				if *updateGoldenFiles {
					gotBytes, err := json.MarshalIndent(expectedOutput, "", "  ")
					require.NoError(t, err, "marshaling converted state to JSON")
					// Newline at end of file for git purposes
					err = os.WriteFile(expPath, append(gotBytes, '\n'), 0o600)
					require.NoError(t, err)
					return
				}

				gotBytes, err := json.Marshal(expectedOutput)
				require.NoError(t, err, "marshaling converted state to JSON")

				expBytes, err := os.ReadFile(expPath)
				require.NoError(t, err)

				require.JSONEq(t, string(expBytes), string(gotBytes), "converted state")
			})
		}
	}
}

// TestConvertStateDeterministic verifies that ConvertState produces
// identical output across multiple runs. This catches non-deterministic
// map iteration in the implementation. Unlike TestConvertStateGolden,
// this test does NOT sort the output — it relies on ConvertState itself
// being deterministic.
func TestConvertStateDeterministic(t *testing.T) {
	t.Parallel()

	testResourceDirectories := filepath.Join("testdata", "resources")
	entries, err := os.ReadDir(testResourceDirectories)
	require.NoError(t, err)

	for _, testDirectory := range entries {
		if !testDirectory.IsDir() {
			continue
		}

		testFiles, err := os.ReadDir(filepath.Join(testResourceDirectories, testDirectory.Name()))
		require.NoError(t, err)

		for _, step := range []string{"plan", "state"} {
			srcIdx := slices.IndexFunc(testFiles, func(entry os.DirEntry) bool {
				return strings.HasSuffix(entry.Name(), fmt.Sprintf(".tf%s.json", step))
			})
			dotIdx := slices.IndexFunc(testFiles, func(entry os.DirEntry) bool {
				return strings.HasSuffix(entry.Name(), fmt.Sprintf(".tf%s.dot", step))
			})

			if srcIdx == -1 || dotIdx == -1 {
				continue
			}

			t.Run(step+"_"+testDirectory.Name(), func(t *testing.T) {
				t.Parallel()
				testDirectoryPath := filepath.Join(testResourceDirectories, testDirectory.Name())
				planFile := filepath.Join(testDirectoryPath, testFiles[srcIdx].Name())
				dotFile := filepath.Join(testDirectoryPath, testFiles[dotIdx].Name())

				ctx := testutil.Context(t, testutil.WaitMedium)
				logger := slogtest.Make(t, nil)

				tfStepRaw, err := os.ReadFile(planFile)
				require.NoError(t, err)

				var modules []*tfjson.StateModule
				switch step {
				case "plan":
					var tfPlan tfjson.Plan
					err = json.Unmarshal(tfStepRaw, &tfPlan)
					require.NoError(t, err)
					modules = []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}
					if tfPlan.PriorState != nil {
						modules = append(modules, tfPlan.PriorState.Values.RootModule)
					}
				case "state":
					var tfState tfjson.State
					err = json.Unmarshal(tfStepRaw, &tfState)
					require.NoError(t, err)
					modules = []*tfjson.StateModule{tfState.Values.RootModule}
				default:
					t.Fatalf("unknown step: %s", step)
				}

				dotFileRaw, err := os.ReadFile(dotFile)
				require.NoError(t, err)

				// Run ConvertState 10 times and verify all runs
				// produce byte-identical JSON without any sorting.
				// We apply deterministicAppIDs because plan files
				// lack provider-assigned IDs, causing ConvertState
				// to generate random UUIDs as a fallback.
				//
				// Note: json.Marshal sorts map keys, so this test
				// cannot catch non-determinism in map-valued fields
				// like Agent.Env. Those are populated from static
				// testdata today, so this is not a practical gap.
				const runs = 10
				outputs := make([][]byte, runs)
				for i := range runs {
					state, err := terraform.ConvertState(ctx, modules, string(dotFileRaw), logger)
					if err != nil {
						// Error strings are deterministic.
						outputs[i] = []byte(err.Error())
						continue
					}
					deterministicAppIDs(state.Resources)
					outputs[i], err = json.Marshal(state)
					require.NoError(t, err, "run %d: marshal state", i)
				}
				for i := 1; i < runs; i++ {
					require.Equal(t, string(outputs[0]), string(outputs[i]),
						"ConvertState produced different output on run %d vs run 0", i)
				}
			})
		}
	}
}

//go:build linux

package terraform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisionersdk/proto"
)

func TestParse(t *testing.T) {
	t.Parallel()

	ctx, api := setupProvisioner(t, nil)

	testCases := []struct {
		Name     string
		Files    map[string]string
		Response *proto.DeprecatedParse_Response
		// If ErrorContains is not empty, then response.Recv() should return an
		// error containing this string before a Complete response is returned.
		ErrorContains string
	}{
		{
			Name: "single-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
			}`,
			},
			Response: &proto.DeprecatedParse_Response{
				Type: &proto.DeprecatedParse_Response_Complete{
					Complete: &proto.DeprecatedParse_Complete{
						ParameterSchemas: []*proto.DeprecatedParameterSchema{{
							Name:                "A",
							RedisplayValue:      true,
							AllowOverrideSource: true,
							Description:         "Testing!",
						}},
					},
				},
			},
		},
		{
			Name: "default-variable-value",
			Files: map[string]string{
				"main.tf": `variable "A" {
				default = "wow"
			}`,
			},
			Response: &proto.DeprecatedParse_Response{
				Type: &proto.DeprecatedParse_Response_Complete{
					Complete: &proto.DeprecatedParse_Complete{
						ParameterSchemas: []*proto.DeprecatedParameterSchema{{
							Name:                "A",
							RedisplayValue:      true,
							AllowOverrideSource: true,
							DefaultSource: &proto.DeprecatedParameterSource{
								Scheme: proto.DeprecatedParameterSource_DATA,
								Value:  "wow",
							},
						}},
					},
				},
			},
		},
		{
			Name: "variable-validation",
			Files: map[string]string{
				"main.tf": `variable "A" {
				validation {
					condition = var.A == "value"
				}
			}`,
			},
			Response: &proto.DeprecatedParse_Response{
				Type: &proto.DeprecatedParse_Response_Complete{
					Complete: &proto.DeprecatedParse_Complete{
						ParameterSchemas: []*proto.DeprecatedParameterSchema{{
							Name:                 "A",
							RedisplayValue:       true,
							ValidationCondition:  `var.A == "value"`,
							ValidationTypeSystem: proto.DeprecatedParameterSchema_HCL,
							AllowOverrideSource:  true,
						}},
					},
				},
			},
		},
		{
			Name: "bad-syntax",
			Files: map[string]string{
				"main.tf": "a;sd;ajsd;lajsd;lasjdf;a",
			},
			ErrorContains: `The ";" character is not valid.`,
		},
		{
			Name: "multiple-variables",
			Files: map[string]string{
				"main1.tf": `variable "foo" { }
				variable "bar" { }`,
				"main2.tf": `variable "baz" { }
				variable "quux" { }`,
			},
			Response: &proto.DeprecatedParse_Response{
				Type: &proto.DeprecatedParse_Response_Complete{
					Complete: &proto.DeprecatedParse_Complete{
						ParameterSchemas: []*proto.DeprecatedParameterSchema{
							{
								Name:                "foo",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
							},
							{
								Name:                "bar",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
							},
							{
								Name:                "baz",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
							},
							{
								Name:                "quux",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			// Write all files to the temporary test directory.
			directory := t.TempDir()
			for path, content := range testCase.Files {
				err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0o600)
				require.NoError(t, err)
			}

			response, err := api.DeprecatedParse(ctx, &proto.DeprecatedParse_Request{
				Directory: directory,
			})
			require.NoError(t, err)

			for {
				msg, err := response.Recv()
				if err != nil {
					if testCase.ErrorContains != "" {
						require.ErrorContains(t, err, testCase.ErrorContains)
						break
					}

					require.NoError(t, err)
				}

				if msg.GetComplete() == nil {
					continue
				}
				if testCase.ErrorContains != "" {
					t.Fatal("expected error but job completed successfully")
				}

				// Ensure the want and got are equivalent!
				want, err := json.Marshal(testCase.Response)
				require.NoError(t, err)
				got, err := json.Marshal(msg)
				require.NoError(t, err)

				require.Equal(t, string(want), string(got))
				break
			}
		})
	}
}

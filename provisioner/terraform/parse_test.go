//go:build linux || darwin

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
		Response *proto.Parse_Response
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
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:                "A",
							RedisplayValue:      true,
							AllowOverrideSource: true,
							Description:         "Testing!",
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
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
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:                "A",
							RedisplayValue:      true,
							AllowOverrideSource: true,
							DefaultSource: &proto.ParameterSource{
								Scheme: proto.ParameterSource_DATA,
								Value:  "wow",
							},
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
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
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:                 "A",
							RedisplayValue:       true,
							ValidationCondition:  `var.A == "value"`,
							ValidationTypeSystem: proto.ParameterSchema_HCL,
							AllowOverrideSource:  true,
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
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
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{
							{
								Name:                "foo",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
							{
								Name:                "bar",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
							{
								Name:                "baz",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
							{
								Name:                "quux",
								RedisplayValue:      true,
								AllowOverrideSource: true,
								Description:         "",
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "enable-managed-variables-with-default-bool",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
				type = 	bool
				default = true
				sensitive = true
			}

			provider "coder" {
				feature_use_managed_variables = true
			}`,
			},
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						TemplateVariables: []*proto.TemplateVariable{
							{
								Name:         "A",
								Description:  "Testing!",
								Type:         "bool",
								DefaultValue: "true",
								Required:     false,
								Sensitive:    true,
							},
						},
					},
				},
			},
		},
		{
			Name: "enable-managed-variables-with-default-string",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
				type = 	string
				default = "abc"
				sensitive = true
			}

			provider "coder" {
				feature_use_managed_variables = true
			}`,
			},
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						TemplateVariables: []*proto.TemplateVariable{
							{
								Name:         "A",
								Description:  "Testing!",
								Type:         "string",
								DefaultValue: "abc",
								Required:     false,
								Sensitive:    true,
							},
						},
					},
				},
			},
		},
		{
			Name: "enable-managed-variables-without-default",
			Files: map[string]string{
				"main2.tf": `variable "A" {
				description = "Testing!"
				type = string
				sensitive = true
			}

			provider "coder" {
				feature_use_managed_variables = true
			}`,
			},
			Response: &proto.Parse_Response{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						TemplateVariables: []*proto.TemplateVariable{
							{
								Name:        "A",
								Description: "Testing!",
								Type:        "string",
								Required:    true,
								Sensitive:   true,
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

			response, err := api.Parse(ctx, &proto.Parse_Request{
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

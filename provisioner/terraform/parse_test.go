//go:build linux || darwin

package terraform_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestParse(t *testing.T) {
	t.Parallel()

	ctx, api := setupProvisioner(t, nil)

	testCases := []struct {
		Name     string
		Files    map[string]string
		Response *proto.ParseComplete
		// If ErrorContains is not empty, then the ParseComplete should have an Error containing the given string
		ErrorContains string
	}{
		{
			Name: "single-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
			}`,
			},
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:        "A",
						Description: "Testing!",
						Required:    true,
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
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:         "A",
						DefaultValue: "wow",
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
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:     "A",
						Required: true,
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
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:     "foo",
						Required: true,
					},
					{
						Name:     "bar",
						Required: true,
					},
					{
						Name:     "baz",
						Required: true,
					},
					{
						Name:     "quux",
						Required: true,
					},
				},
			},
		},
		{
			Name: "template-variables-with-default-bool",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
				type = 	bool
				default = true
				sensitive = true
			}`,
			},
			Response: &proto.ParseComplete{
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
		{
			Name: "template-variables-with-default-string",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
				type = 	string
				default = "abc"
				sensitive = true
			}`,
			},
			Response: &proto.ParseComplete{
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
		{
			Name: "template-variables-with-default-empty-string",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
				type = 	string
				default = ""
				sensitive = true
			}`,
			},
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:         "A",
						Description:  "Testing!",
						Type:         "string",
						DefaultValue: "",
						Required:     false,
						Sensitive:    true,
					},
				},
			},
		},
		{
			Name: "template-variables-without-default",
			Files: map[string]string{
				"main2.tf": `variable "A" {
				description = "Testing!"
				type = string
				sensitive = true
			}`,
			},
			Response: &proto.ParseComplete{
				TemplateVariables: []*proto.TemplateVariable{
					{
						Name:         "A",
						Description:  "Testing!",
						Type:         "string",
						DefaultValue: "",
						Required:     true,
						Sensitive:    true,
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			session := configure(ctx, t, api, &proto.Config{
				TemplateSourceArchive: makeTar(t, testCase.Files),
			})

			err := session.Send(&proto.Request{Type: &proto.Request_Parse{Parse: &proto.ParseRequest{}}})
			require.NoError(t, err)

			for {
				msg, err := session.Recv()
				require.NoError(t, err)

				if testCase.ErrorContains != "" {
					require.Contains(t, msg.GetParse().GetError(), testCase.ErrorContains)
					break
				}

				// Ignore logs in this test
				if msg.GetLog() != nil {
					continue
				}

				// Ensure the want and got are equivalent!
				want, err := json.Marshal(testCase.Response)
				require.NoError(t, err)
				got, err := json.Marshal(msg.GetParse())
				require.NoError(t, err)

				require.Equal(t, string(want), string(got))
				break
			}
		})
	}
}

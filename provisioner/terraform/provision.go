package terraform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// Provision executes `terraform apply`.
func (t *terraform) Provision(stream proto.DRPCProvisioner_ProvisionStream) error {
	shutdown, shutdownFunc := context.WithCancel(stream.Context())
	defer shutdownFunc()

	request, err := stream.Recv()
	if err != nil {
		return err
	}
	if request.GetCancel() != nil {
		return nil
	}
	// We expect the first message is start!
	if request.GetStart() == nil {
		return nil
	}
	go func() {
		for {
			request, err := stream.Recv()
			if err != nil {
				return
			}
			if request.GetCancel() == nil {
				// This is only to process cancels!
				continue
			}
			shutdownFunc()
			return
		}
	}()
	start := request.GetStart()
	statefilePath := filepath.Join(start.Directory, "terraform.tfstate")
	if len(start.State) > 0 {
		err := os.WriteFile(statefilePath, start.State, 0600)
		if err != nil {
			return xerrors.Errorf("write statefile %q: %w", statefilePath, err)
		}
	}

	terraform, err := tfexec.NewTerraform(start.Directory, t.binaryPath)
	if err != nil {
		return xerrors.Errorf("create new terraform executor: %w", err)
	}
	version, _, err := terraform.Version(shutdown, false)
	if err != nil {
		return xerrors.Errorf("get terraform version: %w", err)
	}
	if !version.GreaterThanOrEqual(minimumTerraformVersion) {
		return xerrors.Errorf("terraform version %q is too old. required >= %q", version.String(), minimumTerraformVersion.String())
	}

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: scanner.Text(),
					},
				},
			})
		}
	}()
	terraform.SetStdout(writer)
	t.logger.Debug(shutdown, "running initialization")
	err = terraform.Init(shutdown)
	if err != nil {
		return xerrors.Errorf("initialize terraform: %w", err)
	}
	t.logger.Debug(shutdown, "ran initialization")
	_ = reader.Close()
	terraform.SetStdout(io.Discard)

	env := os.Environ()
	env = append(env,
		"CODER_URL="+start.Metadata.CoderUrl,
		"CODER_WORKSPACE_TRANSITION="+strings.ToLower(start.Metadata.WorkspaceTransition.String()),
	)
	for key, value := range provisionersdk.AgentScriptEnv() {
		env = append(env, key+"="+value)
	}
	vars := []string{}
	for _, param := range start.ParameterValues {
		switch param.DestinationScheme {
		case proto.ParameterDestination_ENVIRONMENT_VARIABLE:
			env = append(env, fmt.Sprintf("%s=%s", param.Name, param.Value))
		case proto.ParameterDestination_PROVISIONER_VARIABLE:
			vars = append(vars, fmt.Sprintf("%s=%s", param.Name, param.Value))
		default:
			return xerrors.Errorf("unsupported parameter type %q for %q", param.DestinationScheme, param.Name)
		}
	}

	closeChan := make(chan struct{})
	reader, writer = io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		defer close(closeChan)
		decoder := json.NewDecoder(reader)
		for {
			var log terraformProvisionLog
			err := decoder.Decode(&log)
			if err != nil {
				return
			}

			logLevel, err := convertTerraformLogLevel(log.Level)
			if err != nil {
				// Not a big deal, but we should handle this at some point!
				continue
			}
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  logLevel,
						Output: log.Message,
					},
				},
			})

			if log.Diagnostic == nil {
				continue
			}

			// If the diagnostic is provided, let's provide a bit more info!
			logLevel, err = convertTerraformLogLevel(log.Diagnostic.Severity)
			if err != nil {
				continue
			}
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  logLevel,
						Output: log.Diagnostic.Detail,
					},
				},
			})
		}
	}()

	planfilePath := filepath.Join(start.Directory, "terraform.tfplan")
	var args []string
	if start.DryRun {
		args = []string{
			"plan",
			"-no-color",
			"-input=false",
			"-json",
			"-refresh=true",
			"-out=" + planfilePath,
		}
	} else {
		args = []string{
			"apply",
			"-no-color",
			"-auto-approve",
			"-input=false",
			"-json",
			"-refresh=true",
		}
	}
	if start.Metadata.WorkspaceTransition == proto.WorkspaceTransition_DESTROY {
		args = append(args, "-destroy")
	}
	for _, variable := range vars {
		args = append(args, "-var", variable)
	}
	// #nosec
	cmd := exec.CommandContext(stream.Context(), t.binaryPath, args...)
	go func() {
		select {
		case <-stream.Context().Done():
			return
		case <-shutdown.Done():
			_ = cmd.Process.Signal(os.Kill)
		}
	}()
	cmd.Stdout = writer
	cmd.Env = env
	cmd.Dir = terraform.WorkingDir()
	err = cmd.Run()
	if err != nil {
		if start.DryRun {
			return xerrors.Errorf("plan terraform: %w", err)
		}
		errorMessage := err.Error()
		// Terraform can fail and apply and still need to store it's state.
		// In this case, we return Complete with an explicit error message.
		statefileContent, err := os.ReadFile(statefilePath)
		if err != nil {
			return xerrors.Errorf("read file %q: %w", statefilePath, err)
		}
		return stream.Send(&proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					State: statefileContent,
					Error: errorMessage,
				},
			},
		})
	}
	_ = reader.Close()
	<-closeChan

	var resp *proto.Provision_Response
	if start.DryRun {
		resp, err = parseTerraformPlan(stream.Context(), terraform, planfilePath)
	} else {
		resp, err = parseTerraformApply(stream.Context(), terraform, statefilePath)
	}
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

func parseTerraformPlan(ctx context.Context, terraform *tfexec.Terraform, planfilePath string) (*proto.Provision_Response, error) {
	plan, err := terraform.ShowPlanFile(ctx, planfilePath)
	if err != nil {
		return nil, xerrors.Errorf("show terraform plan file: %w", err)
	}

	// Maps resource dependencies to expression references.
	// This is *required* for a plan, because "DependsOn"
	// does not propagate.
	resourceDependencies := map[string][]string{}
	for _, resource := range plan.Config.RootModule.Resources {
		if resource.Expressions == nil {
			resource.Expressions = map[string]*tfjson.Expression{}
		}
		// Count expression is separated for logical reasons,
		// but it's simpler syntactically for us to combine here.
		if resource.CountExpression != nil {
			resource.Expressions["count"] = resource.CountExpression
		}
		for _, expression := range resource.Expressions {
			dependencies, exists := resourceDependencies[resource.Address]
			if !exists {
				dependencies = []string{}
			}
			dependencies = append(dependencies, expression.References...)
			resourceDependencies[resource.Address] = dependencies
		}
	}

	resources := make([]*proto.Resource, 0)
	agents := map[string]*proto.Agent{}

	// Store all agents inside the maps!
	for _, resource := range plan.Config.RootModule.Resources {
		if resource.Type != "coder_agent" {
			continue
		}
		agent := &proto.Agent{
			Auth: &proto.Agent_Token{},
		}
		if envRaw, has := resource.Expressions["env"]; has {
			env, ok := envRaw.ConstantValue.(map[string]string)
			if !ok {
				return nil, xerrors.Errorf("unexpected type %q for env map", reflect.TypeOf(envRaw.ConstantValue).String())
			}
			agent.Env = env
		}
		if startupScriptRaw, has := resource.Expressions["startup_script"]; has {
			startupScript, ok := startupScriptRaw.ConstantValue.(string)
			if !ok {
				return nil, xerrors.Errorf("unexpected type %q for startup script", reflect.TypeOf(startupScriptRaw.ConstantValue).String())
			}
			agent.StartupScript = startupScript
		}
		if auth, has := resource.Expressions["auth"]; has {
			if len(auth.ExpressionData.NestedBlocks) > 0 {
				block := auth.ExpressionData.NestedBlocks[0]
				authType, has := block["type"]
				if has {
					authTypeValue, valid := authType.ConstantValue.(string)
					if !valid {
						return nil, xerrors.Errorf("unexpected type %q for auth type", reflect.TypeOf(authType.ConstantValue))
					}
					switch authTypeValue {
					case "google-instance-identity":
						instanceID, _ := block["instance_id"].ConstantValue.(string)
						agent.Auth = &proto.Agent_GoogleInstanceIdentity{
							GoogleInstanceIdentity: &proto.GoogleInstanceIdentityAuth{
								InstanceId: instanceID,
							},
						}
					default:
						return nil, xerrors.Errorf("unknown auth type: %q", authTypeValue)
					}
				}
			}
		}

		agents[resource.Address] = agent
	}

	for _, resource := range plan.PlannedValues.RootModule.Resources {
		if resource.Type == "coder_agent" {
			continue
		}
		// The resource address on planned values can include the indexed
		// value like "[0]", but the config doesn't have these, and we don't
		// care which index the resource is.
		resourceAddress := fmt.Sprintf("%s.%s", resource.Type, resource.Name)
		var agent *proto.Agent
		// Associate resources that depend on an agent.
		for _, dependency := range resourceDependencies[resourceAddress] {
			var has bool
			agent, has = agents[dependency]
			if has {
				break
			}
		}
		// Associate resources where the agent depends on it.
		for agentAddress := range agents {
			for _, depend := range resourceDependencies[agentAddress] {
				if depend != resourceAddress {
					continue
				}
				agent = agents[agentAddress]
				break
			}
		}

		resources = append(resources, &proto.Resource{
			Name:  resource.Name,
			Type:  resource.Type,
			Agent: agent,
		})
	}

	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				Resources: resources,
			},
		},
	}, nil
}

func parseTerraformApply(ctx context.Context, terraform *tfexec.Terraform, statefilePath string) (*proto.Provision_Response, error) {
	statefileContent, err := os.ReadFile(statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("read file %q: %w", statefilePath, err)
	}
	state, err := terraform.ShowStateFile(ctx, statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("show state file %q: %w", statefilePath, err)
	}
	resources := make([]*proto.Resource, 0)
	if state.Values != nil {
		type agentAttributes struct {
			ID    string `mapstructure:"id"`
			Token string `mapstructure:"token"`
			Auth  []struct {
				Type       string `mapstructure:"type"`
				InstanceID string `mapstructure:"instance_id"`
			} `mapstructure:"auth"`
			Env           map[string]string `mapstructure:"env"`
			StartupScript string            `mapstructure:"startup_script"`
		}
		agents := map[string]*proto.Agent{}
		agentDepends := map[string][]string{}

		// Store all agents inside the maps!
		for _, resource := range state.Values.RootModule.Resources {
			if resource.Type != "coder_agent" {
				continue
			}
			var attrs agentAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode agent attributes: %w", err)
			}
			agent := &proto.Agent{
				Id:            attrs.ID,
				Env:           attrs.Env,
				StartupScript: attrs.StartupScript,
				Auth: &proto.Agent_Token{
					Token: attrs.Token,
				},
			}
			if len(attrs.Auth) > 0 {
				auth := attrs.Auth[0]
				switch auth.Type {
				case "google-instance-identity":
					agent.Auth = &proto.Agent_GoogleInstanceIdentity{
						GoogleInstanceIdentity: &proto.GoogleInstanceIdentityAuth{
							InstanceId: auth.InstanceID,
						},
					}
				default:
					return nil, xerrors.Errorf("unknown auth type: %q", auth.Type)
				}
			}
			resourceKey := strings.Join([]string{resource.Type, resource.Name}, ".")
			agents[resourceKey] = agent
			agentDepends[resourceKey] = resource.DependsOn
		}

		for _, resource := range state.Values.RootModule.Resources {
			if resource.Type == "coder_agent" {
				continue
			}
			var agent *proto.Agent
			// Associate resources that depend on an agent.
			for _, dep := range resource.DependsOn {
				var has bool
				agent, has = agents[dep]
				if has {
					break
				}
			}
			if agent == nil {
				// Associate resources where the agent depends on it.
				for agentKey, dependsOn := range agentDepends {
					for _, depend := range dependsOn {
						if depend != strings.Join([]string{resource.Type, resource.Name}, ".") {
							continue
						}
						agent = agents[agentKey]
						break
					}
				}
			}

			if agent != nil {
				if agent.GetGoogleInstanceIdentity() != nil {
					// Make sure the instance has an instance ID!
					_, exists := resource.AttributeValues["instance_id"]
					if !exists {
						// This was a mistake!
						agent = nil
					}
				}
			}

			resources = append(resources, &proto.Resource{
				Name:  resource.Name,
				Type:  resource.Type,
				Agent: agent,
			})
		}
	}

	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				State:     statefileContent,
				Resources: resources,
			},
		},
	}, nil
}

type terraformProvisionLog struct {
	Level   string `json:"@level"`
	Message string `json:"@message"`

	Diagnostic *terraformProvisionLogDiagnostic `json:"diagnostic"`
}

type terraformProvisionLogDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

func convertTerraformLogLevel(logLevel string) (proto.LogLevel, error) {
	switch strings.ToLower(logLevel) {
	case "trace":
		return proto.LogLevel_TRACE, nil
	case "debug":
		return proto.LogLevel_DEBUG, nil
	case "info":
		return proto.LogLevel_INFO, nil
	case "warn":
		return proto.LogLevel_WARN, nil
	case "error":
		return proto.LogLevel_ERROR, nil
	default:
		return proto.LogLevel(0), xerrors.Errorf("invalid log level %q", logLevel)
	}
}

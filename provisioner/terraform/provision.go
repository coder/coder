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
	"regexp"
	"runtime"
	"strings"

	"github.com/awalterschulze/gographviz"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

var (
	// noStateRegex is matched against the output from `terraform state show`
	noStateRegex = regexp.MustCompile(`(?i)State read error.*no state`)
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

	statefilePath := filepath.Join(start.Directory, "terraform.tfstate")
	if len(start.State) > 0 {
		err := os.WriteFile(statefilePath, start.State, 0600)
		if err != nil {
			return xerrors.Errorf("write statefile %q: %w", statefilePath, err)
		}
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
						Level:  proto.LogLevel_DEBUG,
						Output: scanner.Text(),
					},
				},
			})
		}
	}()
	terraformEnv := map[string]string{}
	// Required for "terraform init" to find "git" to
	// clone Terraform modules.
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) < 2 {
			continue
		}
		terraformEnv[parts[0]] = parts[1]
	}
	// Only Linux reliably works with the Terraform plugin
	// cache directory. It's unknown why this is.
	if t.cachePath != "" && runtime.GOOS == "linux" {
		terraformEnv["TF_PLUGIN_CACHE_DIR"] = t.cachePath
	}
	err = terraform.SetEnv(terraformEnv)
	if err != nil {
		return xerrors.Errorf("set terraform env: %w", err)
	}
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
		"CODER_AGENT_URL="+start.Metadata.CoderUrl,
		"CODER_WORKSPACE_TRANSITION="+strings.ToLower(start.Metadata.WorkspaceTransition.String()),
		"CODER_WORKSPACE_NAME="+start.Metadata.WorkspaceName,
		"CODER_WORKSPACE_OWNER="+start.Metadata.WorkspaceOwner,
		"CODER_WORKSPACE_ID="+start.Metadata.WorkspaceId,
		"CODER_WORKSPACE_OWNER_ID="+start.Metadata.WorkspaceOwnerId,
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

	// If we're destroying, exit early if there's no state. This is necessary to
	// avoid any cases where a workspace is "locked out" of terraform due to
	// e.g. bad template param values and cannot be deleted. This is just for
	// contingency, in the future we will try harder to prevent workspaces being
	// broken this hard.
	if start.Metadata.WorkspaceTransition == proto.WorkspaceTransition_DESTROY {
		_, err := getTerraformState(shutdown, terraform, statefilePath)
		if xerrors.Is(err, os.ErrNotExist) {
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "The terraform state does not exist, there is nothing to do",
					},
				},
			})

			return stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			})
		}
		if err != nil {
			err = xerrors.Errorf("get terraform state: %w", err)
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Error: err.Error(),
					},
				},
			})

			return err
		}
	}

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
			_ = cmd.Process.Signal(os.Interrupt)
		}
	}()
	cmd.Stdout = writer
	cmd.Env = env
	cmd.Dir = terraform.WorkingDir()
	err = cmd.Run()
	if err != nil {
		if start.DryRun {
			if shutdown.Err() != nil {
				return stream.Send(&proto.Provision_Response{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Error: err.Error(),
						},
					},
				})
			}
			return xerrors.Errorf("plan terraform: %w", err)
		}
		errorMessage := err.Error()
		// Terraform can fail and apply and still need to store it's state.
		// In this case, we return Complete with an explicit error message.
		stateData, _ := os.ReadFile(statefilePath)
		return stream.Send(&proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					State: stateData,
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

	rawGraph, err := terraform.Graph(ctx)
	if err != nil {
		return nil, xerrors.Errorf("graph: %w", err)
	}
	resourceDependencies, err := findDirectDependencies(rawGraph)
	if err != nil {
		return nil, xerrors.Errorf("find dependencies: %w", err)
	}

	resources := make([]*proto.Resource, 0)
	agents := map[string]*proto.Agent{}

	tfResources := make([]*tfjson.ConfigResource, 0)
	var appendResources func(mod *tfjson.ConfigModule)
	appendResources = func(mod *tfjson.ConfigModule) {
		for _, module := range mod.ModuleCalls {
			appendResources(module.Module)
		}
		tfResources = append(tfResources, mod.Resources...)
	}
	appendResources(plan.Config.RootModule)

	// Store all agents inside the maps!
	for _, resource := range tfResources {
		if resource.Type != "coder_agent" {
			continue
		}
		agent := &proto.Agent{
			Name: resource.Name,
			Auth: &proto.Agent_Token{},
		}
		if operatingSystemRaw, has := resource.Expressions["os"]; has {
			operatingSystem, ok := operatingSystemRaw.ConstantValue.(string)
			if ok {
				agent.OperatingSystem = operatingSystem
			}
		}
		if archRaw, has := resource.Expressions["arch"]; has {
			arch, ok := archRaw.ConstantValue.(string)
			if ok {
				agent.Architecture = arch
			}
		}
		if envRaw, has := resource.Expressions["env"]; has {
			env, ok := envRaw.ConstantValue.(map[string]interface{})
			if ok {
				agent.Env = map[string]string{}
				for key, valueRaw := range env {
					value, valid := valueRaw.(string)
					if !valid {
						continue
					}
					agent.Env[key] = value
				}
			}
		}
		if startupScriptRaw, has := resource.Expressions["startup_script"]; has {
			startupScript, ok := startupScriptRaw.ConstantValue.(string)
			if ok {
				agent.StartupScript = startupScript
			}
		}
		if directoryRaw, has := resource.Expressions["dir"]; has {
			dir, ok := directoryRaw.ConstantValue.(string)
			if ok {
				agent.Directory = dir
			}
		}

		agents[convertAddressToLabel(resource.Address)] = agent
	}

	for _, resource := range tfResources {
		if resource.Mode == tfjson.DataResourceMode {
			continue
		}
		if resource.Type == "coder_agent" || resource.Type == "coder_agent_instance" {
			continue
		}
		resources = append(resources, &proto.Resource{
			Name:   resource.Name,
			Type:   resource.Type,
			Agents: findAgents(resourceDependencies, agents, convertAddressToLabel(resource.Address)),
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
	_, err := os.Stat(statefilePath)
	statefileExisted := err == nil

	state, err := getTerraformState(ctx, terraform, statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("get terraform state: %w", err)
	}

	resources := make([]*proto.Resource, 0)
	if state.Values != nil {
		rawGraph, err := terraform.Graph(ctx)
		if err != nil {
			return nil, xerrors.Errorf("graph: %w", err)
		}
		resourceDependencies, err := findDirectDependencies(rawGraph)
		if err != nil {
			return nil, xerrors.Errorf("find dependencies: %w", err)
		}
		type agentAttributes struct {
			Auth            string            `mapstructure:"auth"`
			OperatingSystem string            `mapstructure:"os"`
			Architecture    string            `mapstructure:"arch"`
			Directory       string            `mapstructure:"dir"`
			ID              string            `mapstructure:"id"`
			Token           string            `mapstructure:"token"`
			Env             map[string]string `mapstructure:"env"`
			StartupScript   string            `mapstructure:"startup_script"`
		}
		agents := map[string]*proto.Agent{}

		tfResources := make([]*tfjson.StateResource, 0)
		var appendResources func(resource *tfjson.StateModule)
		appendResources = func(mod *tfjson.StateModule) {
			for _, module := range mod.ChildModules {
				appendResources(module)
			}
			tfResources = append(tfResources, mod.Resources...)
		}
		appendResources(state.Values.RootModule)

		// Store all agents inside the maps!
		for _, resource := range tfResources {
			if resource.Type != "coder_agent" {
				continue
			}
			var attrs agentAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode agent attributes: %w", err)
			}
			agent := &proto.Agent{
				Name:            resource.Name,
				Id:              attrs.ID,
				Env:             attrs.Env,
				StartupScript:   attrs.StartupScript,
				OperatingSystem: attrs.OperatingSystem,
				Architecture:    attrs.Architecture,
				Directory:       attrs.Directory,
			}
			switch attrs.Auth {
			case "token":
				agent.Auth = &proto.Agent_Token{
					Token: attrs.Token,
				}
			default:
				agent.Auth = &proto.Agent_InstanceId{}
			}
			agents[convertAddressToLabel(resource.Address)] = agent
		}

		// Manually associate agents with instance IDs.
		for _, resource := range tfResources {
			if resource.Type != "coder_agent_instance" {
				continue
			}
			agentIDRaw, valid := resource.AttributeValues["agent_id"]
			if !valid {
				continue
			}
			agentID, valid := agentIDRaw.(string)
			if !valid {
				continue
			}
			instanceIDRaw, valid := resource.AttributeValues["instance_id"]
			if !valid {
				continue
			}
			instanceID, valid := instanceIDRaw.(string)
			if !valid {
				continue
			}

			for _, agent := range agents {
				if agent.Id != agentID {
					continue
				}
				agent.Auth = &proto.Agent_InstanceId{
					InstanceId: instanceID,
				}
				break
			}
		}

		for _, resource := range tfResources {
			if resource.Mode == tfjson.DataResourceMode {
				continue
			}
			if resource.Type == "coder_agent" || resource.Type == "coder_agent_instance" {
				continue
			}
			resourceAgents := findAgents(resourceDependencies, agents, convertAddressToLabel(resource.Address))
			for _, agent := range resourceAgents {
				// Didn't use instance identity.
				if agent.GetToken() != "" {
					continue
				}

				key, isValid := map[string]string{
					"google_compute_instance":         "instance_id",
					"aws_instance":                    "id",
					"azurerm_linux_virtual_machine":   "id",
					"azurerm_windows_virtual_machine": "id",
				}[resource.Type]
				if !isValid {
					// The resource type doesn't support
					// automatically setting the instance ID.
					continue
				}
				instanceIDRaw, valid := resource.AttributeValues[key]
				if !valid {
					continue
				}
				instanceID, valid := instanceIDRaw.(string)
				if !valid {
					continue
				}
				agent.Auth = &proto.Agent_InstanceId{
					InstanceId: instanceID,
				}
			}

			resources = append(resources, &proto.Resource{
				Name:   resource.Name,
				Type:   resource.Type,
				Agents: resourceAgents,
			})
		}
	}

	var stateContent []byte
	// We only want to restore state if it's not hosted remotely.
	if statefileExisted {
		stateContent, err = os.ReadFile(statefilePath)
		if err != nil {
			return nil, xerrors.Errorf("read statefile %q: %w", statefilePath, err)
		}
	}

	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				State:     stateContent,
				Resources: resources,
			},
		},
	}, nil
}

// getTerraformState pulls and merges any remote terraform state into the given
// path and reads the merged state. If there is no state, `os.ErrNotExist` will
// be returned.
func getTerraformState(ctx context.Context, terraform *tfexec.Terraform, statefilePath string) (*tfjson.State, error) {
	statefile, err := os.OpenFile(statefilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, xerrors.Errorf("open statefile %q: %w", statefilePath, err)
	}
	defer statefile.Close()

	// #nosec
	cmd := exec.CommandContext(ctx, terraform.ExecPath(), "state", "pull")
	cmd.Dir = terraform.WorkingDir()
	cmd.Stdout = statefile
	err = cmd.Run()
	if err != nil {
		return nil, xerrors.Errorf("pull terraform state: %w", err)
	}

	state, err := terraform.ShowStateFile(ctx, statefilePath)
	if err != nil {
		if noStateRegex.MatchString(err.Error()) {
			return nil, os.ErrNotExist
		}

		return nil, xerrors.Errorf("show terraform state: %w", err)
	}

	return state, nil
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

// findDirectDependencies maps Terraform resources to their children nodes.
// This parses GraphViz output from Terraform which isn't ideal, but seems reliable.
func findDirectDependencies(rawGraph string) (map[string][]string, error) {
	parsedGraph, err := gographviz.ParseString(rawGraph)
	if err != nil {
		return nil, xerrors.Errorf("parse graph: %w", err)
	}
	graph, err := gographviz.NewAnalysedGraph(parsedGraph)
	if err != nil {
		return nil, xerrors.Errorf("analyze graph: %w", err)
	}
	direct := map[string][]string{}
	for _, node := range graph.Nodes.Nodes {
		label, exists := node.Attrs["label"]
		if !exists {
			continue
		}
		label = strings.Trim(label, `"`)
		direct[label] = findDependenciesWithLabels(graph, node.Name)
	}

	return direct, nil
}

// findDependenciesWithLabels recursively finds nodes with labels (resource and data nodes)
// to build a dependency tree.
func findDependenciesWithLabels(graph *gographviz.Graph, nodeName string) []string {
	dependencies := make([]string, 0)
	for destination := range graph.Edges.SrcToDsts[nodeName] {
		dependencyNode, exists := graph.Nodes.Lookup[destination]
		if !exists {
			continue
		}
		label, exists := dependencyNode.Attrs["label"]
		if !exists {
			dependencies = append(dependencies, findDependenciesWithLabels(graph, dependencyNode.Name)...)
			continue
		}
		label = strings.Trim(label, `"`)
		dependencies = append(dependencies, label)
	}
	return dependencies
}

// findAgents recursively searches through resource dependencies
// to find associated agents. Nested is required for indirect
// dependency matching.
func findAgents(resourceDependencies map[string][]string, agents map[string]*proto.Agent, resourceLabel string) []*proto.Agent {
	resourceNode, exists := resourceDependencies[resourceLabel]
	if !exists {
		return []*proto.Agent{}
	}
	// Associate resources that depend on an agent.
	resourceAgents := make([]*proto.Agent, 0)
	for _, dep := range resourceNode {
		var has bool
		agent, has := agents[dep]
		if !has {
			resourceAgents = append(resourceAgents, findAgents(resourceDependencies, agents, dep)...)
			continue
		}
		resourceAgents = append(resourceAgents, agent)
	}
	return resourceAgents
}

// convertAddressToLabel returns the Terraform address without the count
// specifier. eg. "module.ec2_dev.ec2_instance.dev[0]" becomes "module.ec2_dev.ec2_instance.dev"
func convertAddressToLabel(address string) string {
	return strings.Split(address, "[")[0]
}

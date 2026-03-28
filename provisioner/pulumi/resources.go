package pulumi

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	pulumiStackType          = "pulumi:pulumi:Stack"
	pulumiProviderTypePrefix = "pulumi:providers:"

	coderAgentType        = "coder:index/agent:Agent"
	coderAppType          = "coder:index/app:App"
	coderScriptType       = "coder:index/script:Script"
	coderEnvType          = "coder:index/env:Env"
	coderMetadataType     = "coder:index/metadata:Metadata"
	coderParameterType    = "coder:index/parameter:Parameter"
	coderPresetType       = "coder:index/workspacePreset:WorkspacePreset"
	coderExternalAuthType = "coder:index/externalAuth:ExternalAuth"
	coderAITaskType       = "coder:index/aiTask:AiTask"
)

// State is the Pulumi equivalent of terraform.State and intentionally matches
// the same shape so Graph() can treat both provisioners consistently.
type State struct {
	Resources             []*proto.Resource
	Parameters            []*proto.RichParameter
	Presets               []*proto.Preset
	ExternalAuthProviders []*proto.ExternalAuthProviderResource
	AITasks               []*proto.AITask
	HasAITasks            bool
	HasExternalAgents     bool
}

type stackExport struct {
	Version    int             `json:"version"`
	Deployment stackDeployment `json:"deployment"`
}

type stackDeployment struct {
	Manifest  stackManifest      `json:"manifest"`
	Resources []exportedResource `json:"resources"`
}

type stackManifest struct {
	Time    string `json:"time"`
	Magic   string `json:"magic"`
	Version string `json:"version"`
}

type exportedResource struct {
	URN          string         `json:"urn"`
	Type         string         `json:"type"`
	Custom       bool           `json:"custom"`
	Inputs       map[string]any `json:"inputs"`
	Outputs      map[string]any `json:"outputs"`
	Parent       string         `json:"parent"`
	Provider     string         `json:"provider"`
	Dependencies []string       `json:"dependencies"`
}

type agentMetadataAttributes struct {
	Key         string
	DisplayName string
	Script      string
	Interval    int64
	Timeout     int64
	Order       int64
}

type agentDisplayAppsAttributes struct {
	VSCode               bool
	VSCodeInsiders       bool
	WebTerminal          bool
	SSHHelper            bool
	PortForwardingHelper bool
}

type agentResourcesMonitoringAttributes struct {
	Memory  *agentMemoryResourceMonitor
	Volumes []*agentVolumeResourceMonitor
}

type agentMemoryResourceMonitor struct {
	Enabled   bool
	Threshold int32
}

type agentVolumeResourceMonitor struct {
	Path      string
	Enabled   bool
	Threshold int32
}

type appHealthcheckAttributes struct {
	URL       string
	Interval  int32
	Threshold int32
}

type resourceMetadataAttributes struct {
	ResourceID string
	Hide       bool
	Icon       string
	DailyCost  int32
	Items      []*resourceMetadataItem
}

type resourceMetadataItem struct {
	Key       string
	Value     string
	Sensitive bool
	IsNull    bool
}

type richParameterAttributes struct {
	Name                 string
	DisplayName          string
	Description          string
	Type                 string
	Mutable              bool
	DefaultValue         string
	HasDefaultValue      bool
	Icon                 string
	Options              []*richParameterOption
	Validation           *richParameterValidation
	Required             bool
	Order                int32
	Ephemeral            bool
	SpecifiedFormTypeRaw string
}

type richParameterOption struct {
	Name        string
	Description string
	Value       string
	Icon        string
}

type richParameterValidation struct {
	Regex       string
	Error       string
	Min         int32
	HasMin      bool
	Max         int32
	HasMax      bool
	Monotonic   string
	MinDisabled bool
	MaxDisabled bool
}

type presetAttributes struct {
	Name        string
	Description string
	Icon        string
	Default     bool
	Parameters  []*presetParameterAttributes
	Prebuilds   []*presetPrebuildAttributes
}

type presetParameterAttributes struct {
	Name  string
	Value string
}

type presetPrebuildAttributes struct {
	Instances        int32
	ExpirationPolicy *expirationPolicyAttributes
	Scheduling       *schedulingAttributes
}

type expirationPolicyAttributes struct {
	TTL int32
}

type schedulingAttributes struct {
	Timezone string
	Schedule []*scheduleAttributes
}

type scheduleAttributes struct {
	Cron      string
	Instances int32
}

type externalAuthAttributes struct {
	ID       string
	Optional bool
}

type aiTaskAttributes struct {
	ID         string
	AppID      string
	SidebarApp *aiTaskSidebarAppAttributes
}

type aiTaskSidebarAppAttributes struct {
	ID string
}

type resourceBucket struct {
	key               string
	sortURN           string
	source            *exportedResource
	resource          *proto.Resource
	synthetic         bool
	metadataSourceURN string
}

type agentBinding struct {
	source *exportedResource
	agent  *proto.Agent
	bucket *resourceBucket
}

type dependencyOwnerMatch struct {
	bucket *resourceBucket
	agent  *agentBinding
}

type stateConverter struct {
	ctx    context.Context
	logger slog.Logger

	resources []*exportedResource
	byURN     map[string]*exportedResource
	byType    map[string][]*exportedResource

	bucketsByURN map[string]*resourceBucket
	bucketsByID  map[string]*resourceBucket

	syntheticBuckets map[string]*resourceBucket
	syntheticList    []*resourceBucket

	agentsByURN map[string]*agentBinding
	agentsByID  map[string]*agentBinding

	appSlugs map[string]struct{}
}

// ConvertState consumes a `pulumi stack export` JSON document and converts it
// into the resource graph shape consumed by Coder.
//
// Unlike Terraform, Pulumi does not emit a separate DOT graph for ownership
// resolution, so this converter derives relationships from stack-export
// `dependencies` and `parent` links. We prefer explicit IDs first, then walk
// dependencies breadth-first, then walk parents, and only synthesize a bucket
// for agents as a last resort.
func ConvertState(ctx context.Context, rawJSON []byte, logger slog.Logger) (*State, error) {
	if len(bytes.TrimSpace(rawJSON)) == 0 {
		return nil, xerrors.New("pulumi state must not be empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.UseNumber()

	var exported stackExport
	if err := decoder.Decode(&exported); err != nil {
		return nil, xerrors.Errorf("decode pulumi stack export: %w", err)
	}

	resources := make([]*exportedResource, 0, len(exported.Deployment.Resources))
	for i := range exported.Deployment.Resources {
		resource := &exported.Deployment.Resources[i]
		if strings.TrimSpace(resource.URN) == "" {
			return nil, xerrors.New("pulumi resource URN must not be empty")
		}
		if strings.TrimSpace(resource.Type) == "" {
			return nil, xerrors.Errorf("pulumi resource %q type must not be empty", resource.URN)
		}
		if resource.Inputs == nil {
			resource.Inputs = map[string]any{}
		}
		if resource.Outputs == nil {
			resource.Outputs = map[string]any{}
		}
		resource.Dependencies = uniqueSortedStrings(resource.Dependencies)
		resources = append(resources, resource)
	}
	resources = slices.Clone(resources)
	slices.SortFunc(resources, func(a, b *exportedResource) int {
		return cmp.Compare(a.URN, b.URN)
	})

	converter, err := newStateConverter(ctx, logger, resources)
	if err != nil {
		return nil, err
	}

	return converter.convert()
}

func newStateConverter(ctx context.Context, logger slog.Logger, resources []*exportedResource) (*stateConverter, error) {
	converter := &stateConverter{
		ctx:              ctx,
		logger:           logger,
		resources:        resources,
		byURN:            make(map[string]*exportedResource, len(resources)),
		byType:           make(map[string][]*exportedResource),
		bucketsByURN:     make(map[string]*resourceBucket),
		bucketsByID:      map[string]*resourceBucket{},
		syntheticBuckets: map[string]*resourceBucket{},
		agentsByURN:      make(map[string]*agentBinding),
		agentsByID:       map[string]*agentBinding{},
		appSlugs:         map[string]struct{}{},
	}

	for _, resource := range resources {
		if _, exists := converter.byURN[resource.URN]; exists {
			return nil, xerrors.Errorf("duplicate pulumi resource URN %q", resource.URN)
		}
		converter.byURN[resource.URN] = resource
		converter.byType[resource.Type] = append(converter.byType[resource.Type], resource)

		if !isAttachableResource(resource) {
			continue
		}

		bucket := &resourceBucket{
			key:     resource.URN,
			sortURN: resource.URN,
			source:  resource,
			resource: &proto.Resource{
				Name: displayNameForResource(resource),
				Type: resource.Type,
			},
		}
		converter.bucketsByURN[resource.URN] = bucket

		resourceID, ok, err := stringProp(resource, "id")
		if err != nil {
			return nil, xerrors.Errorf("decode attachable resource id for %q: %w", resource.URN, err)
		}
		if ok && resourceID != "" {
			if existing, exists := converter.bucketsByID[resourceID]; exists {
				return nil, xerrors.Errorf("duplicate attachable Pulumi resource id %q for %q and %q", resourceID, existing.source.URN, resource.URN)
			}
			converter.bucketsByID[resourceID] = bucket
		}
	}

	return converter, nil
}

func (c *stateConverter) convert() (*State, error) {
	if err := c.decodeAgents(); err != nil {
		return nil, err
	}
	if err := c.attachAgents(); err != nil {
		return nil, err
	}
	if err := c.attachApps(); err != nil {
		return nil, err
	}
	if err := c.attachEnvs(); err != nil {
		return nil, err
	}
	if err := c.attachScripts(); err != nil {
		return nil, err
	}
	if err := c.attachMetadata(); err != nil {
		return nil, err
	}

	parameters, err := c.decodeParameters()
	if err != nil {
		return nil, err
	}
	presets, err := c.decodePresets(parameters)
	if err != nil {
		return nil, err
	}
	externalAuthProviders, err := c.decodeExternalAuthProviders()
	if err != nil {
		return nil, err
	}
	aiTasks, err := c.decodeAITasks()
	if err != nil {
		return nil, err
	}

	return &State{
		Resources:             c.materializeResources(),
		Parameters:            parameters,
		Presets:               presets,
		ExternalAuthProviders: externalAuthProviders,
		AITasks:               aiTasks,
		HasAITasks:            len(c.byType[coderAITaskType]) > 0,
		HasExternalAgents:     false,
	}, nil
}

func (c *stateConverter) decodeAgents() error {
	agentNames := map[string]struct{}{}
	for _, resource := range c.byType[coderAgentType] {
		agent, err := decodeAgent(resource)
		if err != nil {
			return err
		}
		if agent.Name == "" {
			return xerrors.Errorf("agent name cannot be empty for %q", resource.URN)
		}
		if !provisioner.AgentNameRegex.MatchString(agent.Name) {
			if strings.Contains(agent.Name, "_") {
				return xerrors.Errorf("agent name %q contains underscores which are no longer supported, please use hyphens instead (regex: %q)", agent.Name, provisioner.AgentNameRegex.String())
			}
			return xerrors.Errorf("agent name %q does not match regex %q", agent.Name, provisioner.AgentNameRegex.String())
		}
		lowerName := strings.ToLower(agent.Name)
		if _, exists := agentNames[lowerName]; exists {
			return xerrors.Errorf("duplicate agent name: %s", agent.Name)
		}
		agentNames[lowerName] = struct{}{}

		binding := &agentBinding{source: resource, agent: agent}
		c.agentsByURN[resource.URN] = binding
		if agent.Id != "" {
			if existing, exists := c.agentsByID[agent.Id]; exists {
				return xerrors.Errorf("duplicate Pulumi agent id %q for %q and %q", agent.Id, existing.source.URN, resource.URN)
			}
			c.agentsByID[agent.Id] = binding
		}
	}
	return nil
}

func (c *stateConverter) attachAgents() error {
	for _, resource := range c.byType[coderAgentType] {
		binding := c.agentsByURN[resource.URN]
		if binding == nil {
			return xerrors.Errorf("missing decoded agent binding for %q", resource.URN)
		}
		if _, err := c.resolveAgentBucket(binding, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func (c *stateConverter) resolveAgentBucket(binding *agentBinding, stack map[string]bool) (*resourceBucket, error) {
	if binding == nil {
		return nil, xerrors.New("agent binding must not be nil")
	}
	if binding.bucket != nil {
		return binding.bucket, nil
	}
	if stack[binding.source.URN] {
		return nil, xerrors.Errorf("cyclic Pulumi agent ownership involving %q", binding.source.URN)
	}
	stack[binding.source.URN] = true
	defer delete(stack, binding.source.URN)

	owner, foundOwner, err := c.closestDependencyOwner(binding.source)
	if err != nil {
		return nil, xerrors.Errorf("resolve Pulumi agent %q by dependencies: %w", binding.agent.Name, err)
	}
	if !foundOwner {
		owner, foundOwner, err = c.parentOwner(binding.source)
		if err != nil {
			return nil, xerrors.Errorf("resolve Pulumi agent %q by parent: %w", binding.agent.Name, err)
		}
	}

	if foundOwner {
		bucket, err := c.ownerBucket(owner, stack)
		if err != nil {
			return nil, err
		}
		binding.bucket = bucket
	} else {
		binding.bucket = c.syntheticBucketFor(binding.source)
		c.logger.Warn(c.ctx, "placing Pulumi agent under synthetic resource bucket",
			slog.F("agent_name", binding.agent.Name),
			slog.F("agent_urn", binding.source.URN),
			slog.F("resource_name", binding.bucket.resource.Name),
		)
	}

	binding.bucket.resource.Agents = append(binding.bucket.resource.Agents, binding.agent)
	return binding.bucket, nil
}

func (c *stateConverter) ownerBucket(owner *dependencyOwnerMatch, stack map[string]bool) (*resourceBucket, error) {
	if owner == nil {
		return nil, xerrors.New("dependency owner must not be nil")
	}
	if owner.bucket != nil {
		return owner.bucket, nil
	}
	if owner.agent == nil {
		return nil, xerrors.New("dependency owner must have either a bucket or an agent")
	}
	return c.resolveAgentBucket(owner.agent, stack)
}

func (c *stateConverter) closestDependencyOwner(resource *exportedResource) (*dependencyOwnerMatch, bool, error) {
	return c.closestDependencyMatch(resource, func(candidate *exportedResource) (*dependencyOwnerMatch, bool) {
		if bucket := c.bucketsByURN[candidate.URN]; bucket != nil {
			return &dependencyOwnerMatch{bucket: bucket}, true
		}
		if agent := c.agentsByURN[candidate.URN]; agent != nil {
			return &dependencyOwnerMatch{agent: agent}, true
		}
		return nil, false
	})
}

func (c *stateConverter) parentOwner(resource *exportedResource) (*dependencyOwnerMatch, bool, error) {
	seen := map[string]bool{}
	for parentURN := resource.Parent; parentURN != ""; {
		if seen[parentURN] {
			return nil, false, xerrors.Errorf("cycle in Pulumi parent chain for %q", resource.URN)
		}
		seen[parentURN] = true

		if bucket := c.bucketsByURN[parentURN]; bucket != nil {
			return &dependencyOwnerMatch{bucket: bucket}, true, nil
		}
		if agent := c.agentsByURN[parentURN]; agent != nil {
			return &dependencyOwnerMatch{agent: agent}, true, nil
		}

		parent := c.byURN[parentURN]
		if parent == nil {
			return nil, false, nil
		}
		parentURN = parent.Parent
	}
	return nil, false, nil
}

func (c *stateConverter) attachApps() error {
	for _, resource := range c.byType[coderAppType] {
		binding, found, err := c.resolveAgentForChild(resource, "app")
		if err != nil {
			return err
		}
		if !found {
			continue
		}

		app, err := decodeApp(c.ctx, c.logger, resource)
		if err != nil {
			return err
		}
		if _, exists := c.appSlugs[app.Slug]; exists {
			return xerrors.Errorf("duplicate app slug, they must be unique per template: %q", app.Slug)
		}
		c.appSlugs[app.Slug] = struct{}{}
		binding.agent.Apps = append(binding.agent.Apps, app)
	}
	return nil
}

func (c *stateConverter) attachEnvs() error {
	for _, resource := range c.byType[coderEnvType] {
		binding, found, err := c.resolveAgentForChild(resource, "env")
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		env, err := decodeEnv(resource)
		if err != nil {
			return err
		}
		binding.agent.ExtraEnvs = append(binding.agent.ExtraEnvs, env)
	}
	return nil
}

func (c *stateConverter) attachScripts() error {
	for _, resource := range c.byType[coderScriptType] {
		binding, found, err := c.resolveAgentForChild(resource, "script")
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		script, err := decodeScript(resource)
		if err != nil {
			return err
		}
		binding.agent.Scripts = append(binding.agent.Scripts, script)
	}
	return nil
}

func (c *stateConverter) resolveAgentForChild(resource *exportedResource, kind string) (*agentBinding, bool, error) {
	agentID, hasAgentID, err := stringProp(resource, "agentId", "agent_id")
	if err != nil {
		return nil, false, xerrors.Errorf("decode agent id for Pulumi %s %q: %w", kind, resource.URN, err)
	}
	if hasAgentID && agentID != "" {
		if binding := c.agentsByID[agentID]; binding != nil {
			return binding, true, nil
		}
	}

	binding, found, err := c.closestDependencyAgent(resource)
	if err != nil {
		return nil, false, xerrors.Errorf("resolve Pulumi %s %q by dependencies: %w", kind, resource.URN, err)
	}
	if !found {
		binding, found, err = c.parentAgent(resource)
		if err != nil {
			return nil, false, xerrors.Errorf("resolve Pulumi %s %q by parent: %w", kind, resource.URN, err)
		}
	}
	if found {
		return binding, true, nil
	}

	c.logger.Warn(c.ctx, "skipping Pulumi child resource without owning agent",
		slog.F("kind", kind),
		slog.F("urn", resource.URN),
		slog.F("agent_id", agentID),
	)
	return nil, false, nil
}

func (c *stateConverter) closestDependencyAgent(resource *exportedResource) (*agentBinding, bool, error) {
	match, found, err := c.closestDependencyMatch(resource, func(candidate *exportedResource) (*dependencyOwnerMatch, bool) {
		if agent := c.agentsByURN[candidate.URN]; agent != nil {
			return &dependencyOwnerMatch{agent: agent}, true
		}
		return nil, false
	})
	if err != nil || !found {
		return nil, false, err
	}
	return match.agent, true, nil
}

func (c *stateConverter) parentAgent(resource *exportedResource) (*agentBinding, bool, error) {
	seen := map[string]bool{}
	for parentURN := resource.Parent; parentURN != ""; {
		if seen[parentURN] {
			return nil, false, xerrors.Errorf("cycle in Pulumi parent chain for %q", resource.URN)
		}
		seen[parentURN] = true
		if agent := c.agentsByURN[parentURN]; agent != nil {
			return agent, true, nil
		}
		parent := c.byURN[parentURN]
		if parent == nil {
			return nil, false, nil
		}
		parentURN = parent.Parent
	}
	return nil, false, nil
}

func (c *stateConverter) attachMetadata() error {
	for _, resource := range c.byType[coderMetadataType] {
		bucket, found, err := c.resolveMetadataBucket(resource)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if bucket.metadataSourceURN != "" {
			return xerrors.Errorf("multiple Pulumi metadata resources resolve to %q: %q and %q", bucket.key, bucket.metadataSourceURN, resource.URN)
		}
		metadata, err := decodeResourceMetadata(resource)
		if err != nil {
			return err
		}
		bucket.metadataSourceURN = resource.URN
		bucket.resource.Hide = metadata.Hide
		bucket.resource.Icon = metadata.Icon
		bucket.resource.DailyCost = metadata.DailyCost
		bucket.resource.Metadata = make([]*proto.Resource_Metadata, 0, len(metadata.Items))
		for _, item := range metadata.Items {
			bucket.resource.Metadata = append(bucket.resource.Metadata, &proto.Resource_Metadata{
				Key:       item.Key,
				Value:     item.Value,
				Sensitive: item.Sensitive,
				IsNull:    item.IsNull,
			})
		}
	}
	return nil
}

func (c *stateConverter) resolveMetadataBucket(resource *exportedResource) (*resourceBucket, bool, error) {
	explicitID, hasExplicitID, err := stringProp(resource, "resourceId", "resource_id")
	if err != nil {
		return nil, false, xerrors.Errorf("decode metadata resource id for %q: %w", resource.URN, err)
	}
	if hasExplicitID && explicitID != "" {
		if bucket := c.lookupBucket(explicitID); bucket != nil {
			return bucket, true, nil
		}
	}

	bucket, found, err := c.closestDependencyBucket(resource)
	if err != nil {
		return nil, false, xerrors.Errorf("resolve Pulumi metadata %q by dependencies: %w", resource.URN, err)
	}
	if !found {
		bucket, found, err = c.parentBucket(resource)
		if err != nil {
			return nil, false, xerrors.Errorf("resolve Pulumi metadata %q by parent: %w", resource.URN, err)
		}
	}
	if found {
		return bucket, true, nil
	}

	c.logger.Warn(c.ctx, "skipping Pulumi metadata without owning resource bucket",
		slog.F("urn", resource.URN),
		slog.F("resource_id", explicitID),
	)
	return nil, false, nil
}

func (c *stateConverter) closestDependencyBucket(resource *exportedResource) (*resourceBucket, bool, error) {
	match, found, err := c.closestDependencyMatch(resource, func(candidate *exportedResource) (*dependencyOwnerMatch, bool) {
		if bucket := c.bucketsByURN[candidate.URN]; bucket != nil {
			return &dependencyOwnerMatch{bucket: bucket}, true
		}
		return nil, false
	})
	if err != nil || !found {
		return nil, false, err
	}
	return match.bucket, true, nil
}

func (c *stateConverter) parentBucket(resource *exportedResource) (*resourceBucket, bool, error) {
	seen := map[string]bool{}
	for parentURN := resource.Parent; parentURN != ""; {
		if seen[parentURN] {
			return nil, false, xerrors.Errorf("cycle in Pulumi parent chain for %q", resource.URN)
		}
		seen[parentURN] = true
		if bucket := c.bucketsByURN[parentURN]; bucket != nil {
			return bucket, true, nil
		}
		parent := c.byURN[parentURN]
		if parent == nil {
			return nil, false, nil
		}
		parentURN = parent.Parent
	}
	return nil, false, nil
}

func (c *stateConverter) lookupBucket(key string) *resourceBucket {
	if bucket := c.bucketsByURN[key]; bucket != nil {
		return bucket
	}
	return c.bucketsByID[key]
}

func (c *stateConverter) closestDependencyMatch(resource *exportedResource, match func(*exportedResource) (*dependencyOwnerMatch, bool)) (*dependencyOwnerMatch, bool, error) {
	visited := map[string]bool{resource.URN: true}
	frontier := uniqueSortedStrings(resource.Dependencies)
	for len(frontier) > 0 {
		next := make([]string, 0)
		matches := make([]*dependencyOwnerMatch, 0)
		matchedURNs := make([]string, 0)
		for _, dependencyURN := range frontier {
			if visited[dependencyURN] {
				continue
			}
			visited[dependencyURN] = true
			dependency := c.byURN[dependencyURN]
			if dependency == nil {
				continue
			}
			if owner, ok := match(dependency); ok {
				matches = append(matches, owner)
				matchedURNs = append(matchedURNs, dependency.URN)
				continue
			}
			next = append(next, dependency.Dependencies...)
		}
		if len(matches) == 1 {
			return matches[0], true, nil
		}
		if len(matches) > 1 {
			slices.Sort(matchedURNs)
			return nil, false, xerrors.Errorf("ambiguous Pulumi dependency resolution for %q: %s", resource.URN, strings.Join(matchedURNs, ", "))
		}
		frontier = uniqueSortedStrings(next)
	}
	return nil, false, nil
}

func (c *stateConverter) syntheticBucketFor(resource *exportedResource) *resourceBucket {
	base := c.syntheticBaseResource(resource)
	key := "synthetic:" + base.URN
	if bucket := c.syntheticBuckets[key]; bucket != nil {
		return bucket
	}
	bucket := &resourceBucket{
		key:       key,
		sortURN:   key,
		source:    base,
		synthetic: true,
		resource: &proto.Resource{
			Name: displayNameForResource(base),
			Type: base.Type,
		},
	}
	c.syntheticBuckets[key] = bucket
	c.syntheticList = append(c.syntheticList, bucket)
	return bucket
}

func (c *stateConverter) syntheticBaseResource(resource *exportedResource) *exportedResource {
	root, ok := c.closestDependencyRoot(resource)
	if ok {
		return root
	}
	return resource
}

func (c *stateConverter) closestDependencyRoot(resource *exportedResource) (*exportedResource, bool) {
	visited := map[string]bool{resource.URN: true}
	frontier := uniqueSortedStrings(resource.Dependencies)
	for len(frontier) > 0 {
		next := make([]string, 0)
		roots := make([]*exportedResource, 0)
		for _, dependencyURN := range frontier {
			if visited[dependencyURN] {
				continue
			}
			visited[dependencyURN] = true
			dependency := c.byURN[dependencyURN]
			if dependency == nil {
				continue
			}
			if len(dependency.Dependencies) == 0 {
				roots = append(roots, dependency)
				continue
			}
			next = append(next, dependency.Dependencies...)
		}
		if len(roots) > 0 {
			slices.SortFunc(roots, func(a, b *exportedResource) int {
				return cmp.Compare(a.URN, b.URN)
			})
			return roots[0], true
		}
		frontier = uniqueSortedStrings(next)
	}
	return nil, false
}

func (c *stateConverter) decodeParameters() ([]*proto.RichParameter, error) {
	parameters := make([]*proto.RichParameter, 0, len(c.byType[coderParameterType]))
	parameterNames := map[string]struct{}{}
	for _, resource := range c.byType[coderParameterType] {
		attrs, err := decodeRichParameter(resource)
		if err != nil {
			return nil, err
		}
		if _, exists := parameterNames[attrs.Name]; exists {
			return nil, xerrors.Errorf("coder_parameter names must be unique but %q appears multiple times", attrs.Name)
		}
		parameterNames[attrs.Name] = struct{}{}

		formType, err := convertParameterFormType(attrs.Type, len(attrs.Options), attrs.SpecifiedFormTypeRaw)
		if err != nil {
			return nil, xerrors.Errorf("decode form_type for Pulumi parameter %q: %w", attrs.Name, err)
		}

		parameter := &proto.RichParameter{
			Name:         attrs.Name,
			DisplayName:  attrs.DisplayName,
			Description:  attrs.Description,
			Type:         attrs.Type,
			Mutable:      attrs.Mutable,
			DefaultValue: attrs.DefaultValue,
			Icon:         attrs.Icon,
			Required:     attrs.Required,
			Order:        attrs.Order,
			Ephemeral:    attrs.Ephemeral,
			FormType:     formType,
		}
		if len(attrs.Options) > 0 {
			parameter.Options = make([]*proto.RichParameterOption, 0, len(attrs.Options))
			for _, option := range attrs.Options {
				parameter.Options = append(parameter.Options, &proto.RichParameterOption{
					Name:        option.Name,
					Description: option.Description,
					Value:       option.Value,
					Icon:        option.Icon,
				})
			}
		}
		if attrs.Validation != nil {
			parameter.ValidationRegex = attrs.Validation.Regex
			parameter.ValidationError = attrs.Validation.Error
			if attrs.Validation.HasMin && !attrs.Validation.MinDisabled {
				parameter.ValidationMin = int32Pointer(attrs.Validation.Min)
			}
			if attrs.Validation.HasMax && !attrs.Validation.MaxDisabled {
				parameter.ValidationMax = int32Pointer(attrs.Validation.Max)
			}
			parameter.ValidationMonotonic = attrs.Validation.Monotonic
		}
		parameters = append(parameters, parameter)
	}
	return parameters, nil
}

func (c *stateConverter) decodePresets(parameters []*proto.RichParameter) ([]*proto.Preset, error) {
	knownParameters := make(map[string]struct{}, len(parameters))
	for _, parameter := range parameters {
		knownParameters[parameter.Name] = struct{}{}
	}

	presets := make([]*proto.Preset, 0, len(c.byType[coderPresetType]))
	presetNames := map[string]struct{}{}
	defaultPresets := 0
	for _, resource := range c.byType[coderPresetType] {
		attrs, err := decodePreset(resource)
		if err != nil {
			return nil, err
		}
		if _, exists := presetNames[attrs.Name]; exists {
			return nil, xerrors.Errorf("coder_workspace_preset names must be unique but %q appears multiple times", attrs.Name)
		}
		presetNames[attrs.Name] = struct{}{}

		presetParameters := make([]*proto.PresetParameter, 0, len(attrs.Parameters))
		duplicatedParameterNames := map[string]struct{}{}
		nonExistentParameters := make([]string, 0)
		seenParameterNames := map[string]struct{}{}
		for _, parameter := range attrs.Parameters {
			if _, exists := seenParameterNames[parameter.Name]; exists {
				duplicatedParameterNames[parameter.Name] = struct{}{}
				continue
			}
			seenParameterNames[parameter.Name] = struct{}{}
			if _, exists := knownParameters[parameter.Name]; !exists {
				nonExistentParameters = append(nonExistentParameters, parameter.Name)
			}
			presetParameters = append(presetParameters, &proto.PresetParameter{Name: parameter.Name, Value: parameter.Value})
		}
		if len(duplicatedParameterNames) > 0 {
			return nil, xerrors.Errorf("coder_workspace_preset parameters must be unique but %s appear multiple times", quotedConjunction(mapKeys(duplicatedParameterNames)))
		}
		if len(nonExistentParameters) > 0 {
			slices.Sort(nonExistentParameters)
			c.logger.Warn(c.ctx, "coder_workspace_preset defines preset values for at least one parameter that is not defined by the template",
				slog.F("parameters", strings.Join(nonExistentParameters, ", ")),
			)
		}
		if len(attrs.Prebuilds) > 1 {
			c.logger.Warn(c.ctx, "coder_workspace_preset must have exactly one prebuild block",
				slog.F("preset", attrs.Name),
				slog.F("count", len(attrs.Prebuilds)),
			)
		}

		prebuild := &proto.Prebuild{}
		if len(attrs.Prebuilds) > 0 {
			prebuild.Instances = attrs.Prebuilds[0].Instances
			if attrs.Prebuilds[0].ExpirationPolicy != nil {
				prebuild.ExpirationPolicy = &proto.ExpirationPolicy{Ttl: attrs.Prebuilds[0].ExpirationPolicy.TTL}
			}
			if attrs.Prebuilds[0].Scheduling != nil {
				prebuild.Scheduling = &proto.Scheduling{Timezone: attrs.Prebuilds[0].Scheduling.Timezone}
				prebuild.Scheduling.Schedule = make([]*proto.Schedule, 0, len(attrs.Prebuilds[0].Scheduling.Schedule))
				for _, schedule := range attrs.Prebuilds[0].Scheduling.Schedule {
					prebuild.Scheduling.Schedule = append(prebuild.Scheduling.Schedule, &proto.Schedule{
						Cron:      schedule.Cron,
						Instances: schedule.Instances,
					})
				}
			}
		}

		preset := &proto.Preset{
			Name:        attrs.Name,
			Description: attrs.Description,
			Icon:        attrs.Icon,
			Default:     attrs.Default,
			Parameters:  presetParameters,
			Prebuild:    prebuild,
		}
		if preset.Default {
			defaultPresets++
		}
		presets = append(presets, preset)
	}
	if defaultPresets > 1 {
		return nil, xerrors.Errorf("a maximum of 1 coder_workspace_preset can be marked as default, but %d are set", defaultPresets)
	}
	return presets, nil
}

func (c *stateConverter) decodeExternalAuthProviders() ([]*proto.ExternalAuthProviderResource, error) {
	providersByID := map[string]*proto.ExternalAuthProviderResource{}
	for _, resource := range c.byType[coderExternalAuthType] {
		attrs, err := decodeExternalAuth(resource)
		if err != nil {
			return nil, err
		}
		providersByID[attrs.ID] = &proto.ExternalAuthProviderResource{Id: attrs.ID, Optional: attrs.Optional}
	}
	providers := make([]*proto.ExternalAuthProviderResource, 0, len(providersByID))
	for _, provider := range providersByID {
		providers = append(providers, provider)
	}
	slices.SortFunc(providers, func(a, b *proto.ExternalAuthProviderResource) int {
		return cmp.Compare(a.Id, b.Id)
	})
	return providers, nil
}

func (c *stateConverter) decodeAITasks() ([]*proto.AITask, error) {
	tasks := make([]*proto.AITask, 0, len(c.byType[coderAITaskType]))
	for _, resource := range c.byType[coderAITaskType] {
		attrs, err := decodeAITask(resource)
		if err != nil {
			return nil, err
		}
		task := &proto.AITask{Id: attrs.ID, AppId: attrs.AppID}
		if attrs.SidebarApp != nil && attrs.SidebarApp.ID != "" {
			task.SidebarApp = &proto.AITaskSidebarApp{Id: attrs.SidebarApp.ID}
		} else if attrs.AppID != "" {
			task.SidebarApp = &proto.AITaskSidebarApp{Id: attrs.AppID}
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (c *stateConverter) materializeResources() []*proto.Resource {
	resources := make([]*proto.Resource, 0, len(c.bucketsByURN)+len(c.syntheticList))
	for _, resource := range c.resources {
		if bucket := c.bucketsByURN[resource.URN]; bucket != nil {
			resources = append(resources, bucket.resource)
		}
	}
	slices.SortFunc(c.syntheticList, func(a, b *resourceBucket) int {
		return cmp.Compare(a.sortURN, b.sortURN)
	})
	for _, bucket := range c.syntheticList {
		resources = append(resources, bucket.resource)
	}
	return resources
}

func decodeAgent(resource *exportedResource) (*proto.Agent, error) {
	metadata, err := decodeAgentMetadata(resource)
	if err != nil {
		return nil, err
	}
	displayApps, err := decodeAgentDisplayApps(resource)
	if err != nil {
		return nil, err
	}
	resourcesMonitoring, err := decodeAgentResourcesMonitoring(resource)
	if err != nil {
		return nil, err
	}

	name := displayNameForResource(resource)
	if explicitName, ok, err := stringProp(resource, "name"); err != nil {
		return nil, propertyError(resource, "name", err)
	} else if ok && explicitName != "" {
		name = explicitName
	}

	agent := &proto.Agent{
		Id:                       optionalString(resource, "id"),
		Name:                     name,
		Env:                      optionalStringMap(resource, "env"),
		OperatingSystem:          optionalString(resource, "operatingSystem", "operating_system", "os"),
		Architecture:             optionalString(resource, "architecture", "arch"),
		Directory:                optionalString(resource, "directory", "dir"),
		ConnectionTimeoutSeconds: optionalInt32(resource, "connectionTimeoutSeconds", "connection_timeout_seconds", "connectionTimeout", "connection_timeout"),
		TroubleshootingUrl:       optionalString(resource, "troubleshootingUrl", "troubleshooting_url"),
		MotdFile:                 optionalString(resource, "motdFile", "motd_file"),
		Metadata:                 metadata,
		DisplayApps:              displayApps,
		Order:                    optionalInt64(resource, "order"),
		ResourcesMonitoring:      resourcesMonitoring,
		ApiKeyScope:              optionalString(resource, "apiKeyScope", "api_key_scope"),
	}

	authMode := strings.ToLower(optionalString(resource, "auth"))
	token := optionalString(resource, "token")
	instanceID := optionalString(resource, "instanceId", "instance_id")
	switch {
	case authMode == "token" || token != "":
		agent.Auth = &proto.Agent_Token{Token: token}
	default:
		agent.Auth = &proto.Agent_InstanceId{InstanceId: instanceID}
	}

	return agent, nil
}

func decodeAgentMetadata(resource *exportedResource) ([]*proto.Agent_Metadata, error) {
	items, err := objectSliceProp(resource, "metadata")
	if err != nil {
		return nil, propertyError(resource, "metadata", err)
	}
	metadata := make([]*proto.Agent_Metadata, 0, len(items))
	for _, item := range items {
		attrs := agentMetadataAttributes{
			Key:         optionalStringMapValue(item, "key"),
			DisplayName: optionalStringMapValue(item, "displayName", "display_name"),
			Script:      optionalStringMapValue(item, "script"),
			Interval:    optionalInt64MapValue(item, "interval"),
			Timeout:     optionalInt64MapValue(item, "timeout"),
			Order:       optionalInt64MapValue(item, "order"),
		}
		metadata = append(metadata, &proto.Agent_Metadata{
			Key:         attrs.Key,
			DisplayName: attrs.DisplayName,
			Script:      attrs.Script,
			Interval:    attrs.Interval,
			Timeout:     attrs.Timeout,
			Order:       attrs.Order,
		})
	}
	return metadata, nil
}

func decodeAgentDisplayApps(resource *exportedResource) (*proto.DisplayApps, error) {
	raw, ok := lookupRaw(resource, "displayApps", "display_apps")
	if !ok {
		return provisionersdk.DefaultDisplayApps(), nil
	}
	object, err := objectFromAny(raw)
	if err != nil {
		return nil, propertyError(resource, "displayApps", err)
	}
	attrs := agentDisplayAppsAttributes{
		VSCode:               optionalBoolMapValue(object, "vscode"),
		VSCodeInsiders:       optionalBoolMapValue(object, "vscodeInsiders", "vscode_insiders"),
		WebTerminal:          optionalBoolMapValue(object, "webTerminal", "web_terminal"),
		SSHHelper:            optionalBoolMapValue(object, "sshHelper", "ssh_helper"),
		PortForwardingHelper: optionalBoolMapValue(object, "portForwardingHelper", "port_forwarding_helper"),
	}
	return &proto.DisplayApps{
		Vscode:               attrs.VSCode,
		VscodeInsiders:       attrs.VSCodeInsiders,
		WebTerminal:          attrs.WebTerminal,
		SshHelper:            attrs.SSHHelper,
		PortForwardingHelper: attrs.PortForwardingHelper,
	}, nil
}

func decodeAgentResourcesMonitoring(resource *exportedResource) (*proto.ResourcesMonitoring, error) {
	monitoring := &proto.ResourcesMonitoring{Volumes: []*proto.VolumeResourceMonitor{}}
	raw, ok := lookupRaw(resource, "resourcesMonitoring", "resources_monitoring")
	if !ok {
		return monitoring, nil
	}
	object, err := objectFromAny(raw)
	if err != nil {
		return nil, propertyError(resource, "resourcesMonitoring", err)
	}

	attrs := &agentResourcesMonitoringAttributes{}
	if memory, ok := lookupMapRaw(object, "memory"); ok {
		memoryObject, err := objectFromAny(memory)
		if err != nil {
			return nil, propertyError(resource, "resourcesMonitoring.memory", err)
		}
		attrs.Memory = &agentMemoryResourceMonitor{
			Enabled:   optionalBoolMapValue(memoryObject, "enabled"),
			Threshold: optionalInt32MapValue(memoryObject, "threshold"),
		}
		monitoring.Memory = &proto.MemoryResourceMonitor{Enabled: attrs.Memory.Enabled, Threshold: attrs.Memory.Threshold}
	}
	volumes, err := objectSliceValue(object, "volumes", "volume")
	if err != nil {
		return nil, propertyError(resource, "resourcesMonitoring.volumes", err)
	}
	attrs.Volumes = make([]*agentVolumeResourceMonitor, 0, len(volumes))
	for _, volume := range volumes {
		volumeAttrs := &agentVolumeResourceMonitor{
			Path:      optionalStringMapValue(volume, "path"),
			Enabled:   optionalBoolMapValue(volume, "enabled"),
			Threshold: optionalInt32MapValue(volume, "threshold"),
		}
		attrs.Volumes = append(attrs.Volumes, volumeAttrs)
		monitoring.Volumes = append(monitoring.Volumes, &proto.VolumeResourceMonitor{
			Path:      volumeAttrs.Path,
			Enabled:   volumeAttrs.Enabled,
			Threshold: volumeAttrs.Threshold,
		})
	}
	return monitoring, nil
}

func decodeApp(ctx context.Context, logger slog.Logger, resource *exportedResource) (*proto.App, error) {
	healthcheck, err := decodeAppHealthcheck(resource)
	if err != nil {
		return nil, err
	}

	slug, ok, err := stringProp(resource, "slug")
	if err != nil {
		return nil, propertyError(resource, "slug", err)
	}
	if !ok || slug == "" {
		slug = displayNameForResource(resource)
	}
	if !provisioner.AppSlugRegex.MatchString(slug) {
		return nil, xerrors.Errorf("app slug %q does not match regex %q", slug, provisioner.AppSlugRegex.String())
	}

	id, ok, err := stringProp(resource, "id")
	if err != nil {
		return nil, propertyError(resource, "id", err)
	}
	if !ok || id == "" {
		logger.Warn(ctx, "pulumi coder_app id was unexpectedly empty", slog.F("urn", resource.URN))
		id = uuid.NewString()
	}

	sharingLevel := proto.AppSharingLevel_OWNER
	switch strings.ToLower(optionalString(resource, "share", "sharingLevel", "sharing_level")) {
	case "authenticated":
		sharingLevel = proto.AppSharingLevel_AUTHENTICATED
	case "public":
		sharingLevel = proto.AppSharingLevel_PUBLIC
	}

	openIn := proto.AppOpenIn_SLIM_WINDOW
	if strings.ToLower(optionalString(resource, "openIn", "open_in")) == "tab" {
		openIn = proto.AppOpenIn_TAB
	}

	displayName := optionalString(resource, "displayName", "display_name")
	if displayName == "" {
		displayName = optionalString(resource, "name")
	}

	return &proto.App{
		Id:           id,
		Slug:         slug,
		DisplayName:  displayName,
		Command:      optionalString(resource, "command"),
		Url:          optionalString(resource, "url"),
		Icon:         optionalString(resource, "icon"),
		Subdomain:    optionalBool(resource, "subdomain"),
		Healthcheck:  healthcheck,
		SharingLevel: sharingLevel,
		External:     optionalBool(resource, "external"),
		Order:        optionalInt64(resource, "order"),
		Hidden:       optionalBool(resource, "hidden"),
		OpenIn:       openIn,
		Group:        optionalString(resource, "group"),
		Tooltip:      optionalString(resource, "tooltip"),
	}, nil
}

//nolint:nilnil // Nil healthchecks are valid when the Pulumi resource omits the block.
func decodeAppHealthcheck(resource *exportedResource) (*proto.Healthcheck, error) {
	raw, ok := lookupRaw(resource, "healthcheck")
	if !ok {
		return nil, nil
	}
	object, err := objectFromAny(raw)
	if err != nil {
		return nil, propertyError(resource, "healthcheck", err)
	}
	attrs := appHealthcheckAttributes{
		URL:       optionalStringMapValue(object, "url"),
		Interval:  optionalInt32MapValue(object, "interval"),
		Threshold: optionalInt32MapValue(object, "threshold"),
	}
	return &proto.Healthcheck{Url: attrs.URL, Interval: attrs.Interval, Threshold: attrs.Threshold}, nil
}

func decodeEnv(resource *exportedResource) (*proto.Env, error) {
	name, ok, err := stringProp(resource, "name")
	if err != nil {
		return nil, propertyError(resource, "name", err)
	}
	if !ok || name == "" {
		return nil, xerrors.Errorf("Pulumi env %q name must not be empty", resource.URN)
	}
	value, ok, err := stringProp(resource, "value")
	if err != nil {
		return nil, propertyError(resource, "value", err)
	}
	if !ok {
		value = ""
	}
	return &proto.Env{
		Name:          name,
		Value:         value,
		MergeStrategy: optionalString(resource, "mergeStrategy", "merge_strategy"),
	}, nil
}

func decodeScript(resource *exportedResource) (*proto.Script, error) {
	return &proto.Script{
		DisplayName:      optionalString(resource, "displayName", "display_name"),
		Icon:             optionalString(resource, "icon"),
		Script:           optionalString(resource, "script"),
		Cron:             optionalString(resource, "cron"),
		StartBlocksLogin: optionalBool(resource, "startBlocksLogin", "start_blocks_login"),
		RunOnStart:       optionalBool(resource, "runOnStart", "run_on_start"),
		RunOnStop:        optionalBool(resource, "runOnStop", "run_on_stop"),
		TimeoutSeconds:   optionalInt32(resource, "timeoutSeconds", "timeout_seconds", "timeout"),
		LogPath:          optionalString(resource, "logPath", "log_path"),
	}, nil
}

func decodeResourceMetadata(resource *exportedResource) (*resourceMetadataAttributes, error) {
	attrs := &resourceMetadataAttributes{
		ResourceID: optionalString(resource, "resourceId", "resource_id"),
		Hide:       optionalBool(resource, "hide"),
		Icon:       optionalString(resource, "icon"),
		DailyCost:  optionalInt32(resource, "dailyCost", "daily_cost"),
		Items:      []*resourceMetadataItem{},
	}
	items, err := objectSliceProp(resource, "items", "item")
	if err != nil {
		return nil, propertyError(resource, "items", err)
	}
	for _, item := range items {
		attrs.Items = append(attrs.Items, &resourceMetadataItem{
			Key:       optionalStringMapValue(item, "key"),
			Value:     optionalStringMapValue(item, "value"),
			Sensitive: optionalBoolMapValue(item, "sensitive"),
			IsNull:    optionalBoolMapValue(item, "isNull", "is_null"),
		})
	}
	return attrs, nil
}

func decodeRichParameter(resource *exportedResource) (*richParameterAttributes, error) {
	name, ok, err := stringProp(resource, "name")
	if err != nil {
		return nil, propertyError(resource, "name", err)
	}
	if !ok || name == "" {
		return nil, xerrors.Errorf("Pulumi parameter %q name must not be empty", resource.URN)
	}
	paramType := optionalString(resource, "type")
	if paramType == "" {
		paramType = "string"
	}
	attrs := &richParameterAttributes{
		Name:                 name,
		DisplayName:          optionalString(resource, "displayName", "display_name"),
		Description:          optionalString(resource, "description"),
		Type:                 paramType,
		Mutable:              optionalBool(resource, "mutable"),
		Icon:                 optionalString(resource, "icon"),
		Order:                optionalInt32(resource, "order"),
		Ephemeral:            optionalBool(resource, "ephemeral"),
		SpecifiedFormTypeRaw: optionalString(resource, "formType", "form_type"),
	}
	if defaultValue, ok, err := stringProp(resource, "default", "defaultValue", "default_value"); err != nil {
		return nil, propertyError(resource, "default", err)
	} else if ok {
		attrs.DefaultValue = defaultValue
		attrs.HasDefaultValue = true
	}

	required, hasRequired, err := boolProp(resource, "required")
	if err != nil {
		return nil, propertyError(resource, "required", err)
	}
	if hasRequired {
		attrs.Required = required
	} else {
		optional, hasOptional, err := boolProp(resource, "optional")
		if err != nil {
			return nil, propertyError(resource, "optional", err)
		}
		if hasOptional {
			attrs.Required = !optional
		} else {
			attrs.Required = !attrs.HasDefaultValue
		}
	}

	options, err := objectSliceProp(resource, "options", "option")
	if err != nil {
		return nil, propertyError(resource, "options", err)
	}
	attrs.Options = make([]*richParameterOption, 0, len(options))
	for _, option := range options {
		attrs.Options = append(attrs.Options, &richParameterOption{
			Name:        optionalStringMapValue(option, "name"),
			Description: optionalStringMapValue(option, "description"),
			Value:       optionalStringMapValue(option, "value"),
			Icon:        optionalStringMapValue(option, "icon"),
		})
	}

	validationRaw, ok := lookupRaw(resource, "validation")
	if !ok {
		return attrs, nil
	}
	validationObject, err := objectFromAny(validationRaw)
	if err != nil {
		return nil, propertyError(resource, "validation", err)
	}
	validation := &richParameterValidation{
		Regex:       optionalStringMapValue(validationObject, "regex"),
		Error:       optionalStringMapValue(validationObject, "error"),
		Monotonic:   optionalStringMapValue(validationObject, "monotonic"),
		MinDisabled: optionalBoolMapValue(validationObject, "minDisabled", "min_disabled"),
		MaxDisabled: optionalBoolMapValue(validationObject, "maxDisabled", "max_disabled"),
	}
	if minValue, ok, err := int32MapValue(validationObject, "min"); err != nil {
		return nil, propertyError(resource, "validation.min", err)
	} else if ok {
		validation.Min = minValue
		validation.HasMin = true
	}
	if maxValue, ok, err := int32MapValue(validationObject, "max"); err != nil {
		return nil, propertyError(resource, "validation.max", err)
	} else if ok {
		validation.Max = maxValue
		validation.HasMax = true
	}
	if !hasAnyMapKey(validationObject, "minDisabled", "min_disabled") {
		validation.MinDisabled = !validation.HasMin
	}
	if !hasAnyMapKey(validationObject, "maxDisabled", "max_disabled") {
		validation.MaxDisabled = !validation.HasMax
	}
	attrs.Validation = validation
	return attrs, nil
}

func decodePreset(resource *exportedResource) (*presetAttributes, error) {
	name, ok, err := stringProp(resource, "name")
	if err != nil {
		return nil, propertyError(resource, "name", err)
	}
	if !ok || name == "" {
		return nil, xerrors.Errorf("Pulumi preset %q name must not be empty", resource.URN)
	}
	attrs := &presetAttributes{
		Name:        name,
		Description: optionalString(resource, "description"),
		Icon:        optionalString(resource, "icon"),
		Default:     optionalBool(resource, "default"),
		Parameters:  []*presetParameterAttributes{},
		Prebuilds:   []*presetPrebuildAttributes{},
	}

	parametersRaw, ok := lookupRaw(resource, "parameters")
	if ok {
		switch typed := parametersRaw.(type) {
		case map[string]any:
			names := mapKeys(typed)
			slices.Sort(names)
			for _, parameterName := range names {
				value, err := stringFromAny(typed[parameterName])
				if err != nil {
					return nil, propertyError(resource, fmt.Sprintf("parameters.%s", parameterName), err)
				}
				attrs.Parameters = append(attrs.Parameters, &presetParameterAttributes{Name: parameterName, Value: value})
			}
		default:
			parameterObjects, err := objectSliceFromAny(parametersRaw)
			if err != nil {
				return nil, propertyError(resource, "parameters", err)
			}
			for _, parameter := range parameterObjects {
				attrs.Parameters = append(attrs.Parameters, &presetParameterAttributes{
					Name:  optionalStringMapValue(parameter, "name"),
					Value: optionalStringMapValue(parameter, "value"),
				})
			}
		}
	}

	prebuildObjects, err := objectSliceProp(resource, "prebuilds", "prebuild")
	if err != nil {
		return nil, propertyError(resource, "prebuilds", err)
	}
	for _, prebuild := range prebuildObjects {
		prebuildAttrs := &presetPrebuildAttributes{Instances: optionalInt32MapValue(prebuild, "instances")}
		if expiration, ok := lookupMapRaw(prebuild, "expirationPolicy", "expiration_policy"); ok {
			expirationObject, err := objectFromAny(expiration)
			if err != nil {
				return nil, propertyError(resource, "prebuilds.expirationPolicy", err)
			}
			prebuildAttrs.ExpirationPolicy = &expirationPolicyAttributes{TTL: optionalInt32MapValue(expirationObject, "ttl")}
		}
		if scheduling, ok := lookupMapRaw(prebuild, "scheduling"); ok {
			schedulingObject, err := objectFromAny(scheduling)
			if err != nil {
				return nil, propertyError(resource, "prebuilds.scheduling", err)
			}
			schedulingAttrs := &schedulingAttributes{
				Timezone: optionalStringMapValue(schedulingObject, "timezone"),
				Schedule: []*scheduleAttributes{},
			}
			schedules, err := objectSliceValue(schedulingObject, "schedule")
			if err != nil {
				return nil, propertyError(resource, "prebuilds.scheduling.schedule", err)
			}
			for _, schedule := range schedules {
				schedulingAttrs.Schedule = append(schedulingAttrs.Schedule, &scheduleAttributes{
					Cron:      optionalStringMapValue(schedule, "cron"),
					Instances: optionalInt32MapValue(schedule, "instances"),
				})
			}
			prebuildAttrs.Scheduling = schedulingAttrs
		}
		attrs.Prebuilds = append(attrs.Prebuilds, prebuildAttrs)
	}

	return attrs, nil
}

func decodeExternalAuth(resource *exportedResource) (*externalAuthAttributes, error) {
	id, ok, err := stringProp(resource, "id")
	if err != nil {
		return nil, propertyError(resource, "id", err)
	}
	if !ok || id == "" {
		return nil, xerrors.Errorf("Pulumi external auth %q id must not be empty", resource.URN)
	}
	return &externalAuthAttributes{ID: id, Optional: optionalBool(resource, "optional")}, nil
}

func decodeAITask(resource *exportedResource) (*aiTaskAttributes, error) {
	id, ok, err := stringProp(resource, "id")
	if err != nil {
		return nil, propertyError(resource, "id", err)
	}
	if !ok || id == "" {
		return nil, xerrors.Errorf("Pulumi AI task %q id must not be empty", resource.URN)
	}
	attrs := &aiTaskAttributes{ID: id}
	appID := optionalString(resource, "appId", "app_id")
	if sidebarRaw, ok := lookupRaw(resource, "sidebarApp", "sidebar_app"); ok {
		sidebarObject, err := objectFromAny(sidebarRaw)
		if err != nil {
			return nil, propertyError(resource, "sidebarApp", err)
		}
		attrs.SidebarApp = &aiTaskSidebarAppAttributes{ID: optionalStringMapValue(sidebarObject, "id")}
		if appID == "" {
			appID = attrs.SidebarApp.ID
		}
	}
	attrs.AppID = appID
	return attrs, nil
}

func propertyError(resource *exportedResource, field string, err error) error {
	return xerrors.Errorf("decode Pulumi resource %q (%s) field %q: %w", resource.URN, resource.Type, field, err)
}

func isAttachableResource(resource *exportedResource) bool {
	return resource.Custom && !isPulumiInternalResourceType(resource.Type) && !isCoderResourceType(resource.Type)
}

func isPulumiInternalResourceType(resourceType string) bool {
	return resourceType == pulumiStackType || strings.HasPrefix(resourceType, pulumiProviderTypePrefix)
}

func isCoderResourceType(resourceType string) bool {
	switch resourceType {
	case coderAgentType,
		coderAppType,
		coderScriptType,
		coderEnvType,
		coderMetadataType,
		coderParameterType,
		coderPresetType,
		coderExternalAuthType,
		coderAITaskType:
		return true
	default:
		return false
	}
}

func displayNameForResource(resource *exportedResource) string {
	if name := urnName(resource.URN); name != "" {
		return name
	}
	if name, ok, err := stringProp(resource, "name"); err == nil && ok && name != "" {
		return name
	}
	return resource.Type
}

func urnName(urn string) string {
	index := strings.LastIndex(urn, "::")
	if index == -1 || index+2 >= len(urn) {
		return ""
	}
	return urn[index+2:]
}

func lookupRaw(resource *exportedResource, keys ...string) (any, bool) {
	for _, properties := range []map[string]any{resource.Outputs, resource.Inputs} {
		for _, key := range keys {
			if value, ok := properties[key]; ok {
				return value, true
			}
		}
	}
	return nil, false
}

func lookupMapRaw(properties map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := properties[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func stringProp(resource *exportedResource, keys ...string) (string, bool, error) {
	value, ok := lookupRaw(resource, keys...)
	if !ok {
		return "", false, nil
	}
	stringValue, err := stringFromAny(value)
	if err != nil {
		return "", false, err
	}
	return stringValue, true, nil
}

func boolProp(resource *exportedResource, keys ...string) (value bool, ok bool, err error) {
	raw, ok := lookupRaw(resource, keys...)
	if !ok {
		return false, false, nil
	}
	value, err = boolFromAny(raw)
	if err != nil {
		return false, false, err
	}
	return value, true, nil
}

func int32Prop(resource *exportedResource, keys ...string) (int32, bool, error) {
	value, ok := lookupRaw(resource, keys...)
	if !ok {
		return 0, false, nil
	}
	intValue, err := int32FromAny(value)
	if err != nil {
		return 0, false, err
	}
	return intValue, true, nil
}

func int64Prop(resource *exportedResource, keys ...string) (int64, bool, error) {
	value, ok := lookupRaw(resource, keys...)
	if !ok {
		return 0, false, nil
	}
	intValue, err := int64FromAny(value)
	if err != nil {
		return 0, false, err
	}
	return intValue, true, nil
}

func stringMapProp(resource *exportedResource, keys ...string) (map[string]string, bool, error) {
	value, ok := lookupRaw(resource, keys...)
	if !ok {
		return nil, false, nil
	}
	stringMap, err := stringMapFromAny(value)
	if err != nil {
		return nil, false, err
	}
	return stringMap, true, nil
}

func objectSliceProp(resource *exportedResource, keys ...string) ([]map[string]any, error) {
	value, ok := lookupRaw(resource, keys...)
	if !ok {
		return nil, nil
	}
	return objectSliceFromAny(value)
}

func objectSliceValue(properties map[string]any, keys ...string) ([]map[string]any, error) {
	value, ok := lookupMapRaw(properties, keys...)
	if !ok {
		return nil, nil
	}
	return objectSliceFromAny(value)
}

func optionalString(resource *exportedResource, keys ...string) string {
	value, ok, err := stringProp(resource, keys...)
	if err != nil || !ok {
		return ""
	}
	return value
}

func optionalBool(resource *exportedResource, keys ...string) bool {
	value, ok, err := boolProp(resource, keys...)
	if err != nil || !ok {
		return false
	}
	return value
}

func optionalInt32(resource *exportedResource, keys ...string) int32 {
	value, ok, err := int32Prop(resource, keys...)
	if err != nil || !ok {
		return 0
	}
	return value
}

func optionalInt64(resource *exportedResource, keys ...string) int64 {
	value, ok, err := int64Prop(resource, keys...)
	if err != nil || !ok {
		return 0
	}
	return value
}

func optionalStringMap(resource *exportedResource, keys ...string) map[string]string {
	value, ok, err := stringMapProp(resource, keys...)
	if err != nil || !ok {
		return nil
	}
	return value
}

func optionalStringMapValue(properties map[string]any, keys ...string) string {
	value, ok, err := stringMapValue(properties, keys...)
	if err != nil || !ok {
		return ""
	}
	return value
}

func optionalBoolMapValue(properties map[string]any, keys ...string) bool {
	value, ok, err := boolMapValue(properties, keys...)
	if err != nil || !ok {
		return false
	}
	return value
}

func optionalInt32MapValue(properties map[string]any, keys ...string) int32 {
	value, ok, err := int32MapValue(properties, keys...)
	if err != nil || !ok {
		return 0
	}
	return value
}

func optionalInt64MapValue(properties map[string]any, keys ...string) int64 {
	value, ok, err := int64MapValue(properties, keys...)
	if err != nil || !ok {
		return 0
	}
	return value
}

func stringMapValue(properties map[string]any, keys ...string) (string, bool, error) {
	value, ok := lookupMapRaw(properties, keys...)
	if !ok {
		return "", false, nil
	}
	stringValue, err := stringFromAny(value)
	if err != nil {
		return "", false, err
	}
	return stringValue, true, nil
}

func boolMapValue(properties map[string]any, keys ...string) (value bool, ok bool, err error) {
	raw, ok := lookupMapRaw(properties, keys...)
	if !ok {
		return false, false, nil
	}
	value, err = boolFromAny(raw)
	if err != nil {
		return false, false, err
	}
	return value, true, nil
}

func int32MapValue(properties map[string]any, keys ...string) (int32, bool, error) {
	value, ok := lookupMapRaw(properties, keys...)
	if !ok {
		return 0, false, nil
	}
	intValue, err := int32FromAny(value)
	if err != nil {
		return 0, false, err
	}
	return intValue, true, nil
}

func int64MapValue(properties map[string]any, keys ...string) (int64, bool, error) {
	value, ok := lookupMapRaw(properties, keys...)
	if !ok {
		return 0, false, nil
	}
	intValue, err := int64FromAny(value)
	if err != nil {
		return 0, false, err
	}
	return intValue, true, nil
}

func stringFromAny(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case nil:
		return "", nil
	default:
		return "", xerrors.Errorf("expected string-compatible value, got %T", value)
	}
}

func boolFromAny(value any) (bool, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err != nil {
			return false, xerrors.Errorf("parse bool %q: %w", typed, err)
		}
		return parsed, nil
	case json.Number:
		intValue, err := typed.Int64()
		if err != nil {
			return false, xerrors.Errorf("parse numeric bool %q: %w", typed.String(), err)
		}
		return intValue != 0, nil
	default:
		return false, xerrors.Errorf("expected bool-compatible value, got %T", value)
	}
}

func int32FromAny(value any) (int32, error) {
	intValue, err := int64FromAny(value)
	if err != nil {
		return 0, err
	}
	if intValue < math.MinInt32 || intValue > math.MaxInt32 {
		return 0, xerrors.Errorf("value %d overflows int32", intValue)
	}
	return int32(intValue), nil
}

func int64FromAny(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		if typed > math.MaxInt64 {
			return 0, xerrors.Errorf("value %d overflows int64", typed)
		}
		return int64(typed), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, xerrors.Errorf("value %d overflows int64", typed)
		}
		return int64(typed), nil
	case float32:
		return int64FromAny(float64(typed))
	case float64:
		if math.Trunc(typed) != typed {
			return 0, xerrors.Errorf("expected integral number, got %v", typed)
		}
		if typed < math.MinInt64 || typed > math.MaxInt64 {
			return 0, xerrors.Errorf("value %v overflows int64", typed)
		}
		return int64(typed), nil
	case json.Number:
		if intValue, err := typed.Int64(); err == nil {
			return intValue, nil
		}
		floatValue, err := typed.Float64()
		if err != nil {
			return 0, xerrors.Errorf("parse number %q: %w", typed.String(), err)
		}
		return int64FromAny(floatValue)
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, xerrors.Errorf("parse integer %q: %w", typed, err)
		}
		return parsed, nil
	default:
		return 0, xerrors.Errorf("expected integer-compatible value, got %T", value)
	}
}

func stringMapFromAny(value any) (map[string]string, error) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, xerrors.Errorf("expected object, got %T", value)
	}
	result := make(map[string]string, len(typed))
	names := mapKeys(typed)
	slices.Sort(names)
	for _, key := range names {
		stringValue, err := stringFromAny(typed[key])
		if err != nil {
			return nil, xerrors.Errorf("decode key %q: %w", key, err)
		}
		result[key] = stringValue
	}
	return result, nil
}

func objectFromAny(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, nil
	case []any:
		if len(typed) == 0 {
			return nil, xerrors.New("expected object, got empty array")
		}
		object, ok := typed[0].(map[string]any)
		if !ok {
			return nil, xerrors.Errorf("expected object array element, got %T", typed[0])
		}
		return object, nil
	default:
		return nil, xerrors.Errorf("expected object, got %T", value)
	}
}

func objectSliceFromAny(value any) ([]map[string]any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return []map[string]any{typed}, nil
	case []any:
		objects := make([]map[string]any, 0, len(typed))
		for i, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, xerrors.Errorf("expected object at index %d, got %T", i, item)
			}
			objects = append(objects, object)
		}
		return objects, nil
	default:
		return nil, xerrors.Errorf("expected object or object array, got %T", value)
	}
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := slices.Clone(values)
	slices.Sort(cloned)
	return slices.Compact(cloned)
}

func mapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func hasAnyMapKey(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func quotedConjunction(values []string) string {
	if len(values) == 0 {
		return ""
	}
	slices.Sort(values)
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", and " + quoted[len(quoted)-1]
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}

func convertParameterFormType(parameterType string, optionCount int, rawFormType string) (proto.ParameterFormType, error) {
	normalized := strings.ToLower(strings.TrimSpace(rawFormType))
	allowed := allowedFormTypes(parameterType, optionCount)
	if len(allowed) == 0 {
		return proto.ParameterFormType_DEFAULT, xerrors.Errorf("unsupported parameter type %q", parameterType)
	}
	if normalized == "" {
		normalized = allowed[0]
	}
	if !slices.Contains(allowed, normalized) {
		return proto.ParameterFormType_DEFAULT, xerrors.Errorf("form_type %q is not supported for type %q, choose one of %v", normalized, parameterType, allowed)
	}
	return protoFormType(normalized)
}

func allowedFormTypes(parameterType string, optionCount int) []string {
	hasOptions := optionCount > 0
	switch parameterType {
	case "string":
		if hasOptions {
			return []string{"radio", "dropdown"}
		}
		return []string{"input", "textarea"}
	case "number":
		if hasOptions {
			return []string{"radio", "dropdown"}
		}
		return []string{"input", "slider"}
	case "bool":
		if hasOptions {
			return []string{"radio", "dropdown"}
		}
		return []string{"checkbox", "switch"}
	case "list(string)":
		if hasOptions {
			return []string{"radio", "multi-select"}
		}
		return []string{"tag-select"}
	default:
		return nil
	}
}

func protoFormType(raw string) (proto.ParameterFormType, error) {
	switch raw {
	case "", "default":
		return proto.ParameterFormType_DEFAULT, nil
	case "error":
		return proto.ParameterFormType_FORM_ERROR, nil
	case "radio":
		return proto.ParameterFormType_RADIO, nil
	case "dropdown":
		return proto.ParameterFormType_DROPDOWN, nil
	case "input":
		return proto.ParameterFormType_INPUT, nil
	case "textarea":
		return proto.ParameterFormType_TEXTAREA, nil
	case "slider":
		return proto.ParameterFormType_SLIDER, nil
	case "checkbox":
		return proto.ParameterFormType_CHECKBOX, nil
	case "switch":
		return proto.ParameterFormType_SWITCH, nil
	case "tag-select":
		return proto.ParameterFormType_TAGSELECT, nil
	case "multi-select":
		return proto.ParameterFormType_MULTISELECT, nil
	default:
		return proto.ParameterFormType_DEFAULT, xerrors.Errorf("unsupported form type: %s", raw)
	}
}

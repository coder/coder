package codersdk

// FeatureStage represents the maturity level of a feature
type FeatureStage string

const (
	// FeatureStageEarlyAccess indicates a feature that is neither feature-complete nor stable
	// Early access features are often disabled by default and not recommended for production use
	FeatureStageEarlyAccess FeatureStage = "early access"
	
	// FeatureStageBeta indicates a feature that is open to the public but still under development
	// Beta features might have minor bugs but are generally ready for use
	FeatureStageBeta FeatureStage = "beta"
)

// Feature contains metadata about a Coder feature
type Feature struct {
	// Name is the display name of the feature
	Name string `json:"name"`
	
	// Description provides details about the feature
	Description string `json:"description"`
	
	// Stage indicates the current maturity level
	Stage FeatureStage `json:"stage"`
	
	// DocsPath is the relative path to the feature's documentation
	DocsPath string `json:"docs_path"`
	
	// Experiment is the associated experiment flag (if applicable)
	// An empty value means no experiment flag is associated
	Experiment Experiment `json:"experiment,omitempty"`
}

// FeatureRegistry maps documentation paths to their feature stages
// This is the central registry for feature stage information
var FeatureRegistry = map[string]Feature{
	// Early Access features
	"user-guides/devcontainers/index.md": {
		Name:        "Dev Containers Integration",
		Description: "Run containerized development environments in your Coder workspace using the dev containers specification.",
		Stage:       FeatureStageEarlyAccess,
		Experiment:  ExperimentDevContainers,
	},
	"user-guides/devcontainers/working-with-dev-containers.md": {
		Name:        "Working with Dev Containers",
		Description: "Access dev containers via SSH, your IDE, or web terminal.",
		Stage:       FeatureStageEarlyAccess,
		Experiment:  ExperimentDevContainers,
	},
	"user-guides/devcontainers/troubleshooting-dev-containers.md": {
		Name:        "Troubleshooting Dev Containers",
		Description: "Diagnose and resolve common issues with dev containers in your Coder workspace.",
		Stage:       FeatureStageEarlyAccess,
		Experiment:  ExperimentDevContainers,
	},
	"admin/templates/extending-templates/devcontainers.md": {
		Name:        "Configure a template for dev containers",
		Description: "How to configure your template for dev containers",
		Stage:       FeatureStageEarlyAccess,
		Experiment:  ExperimentDevContainers,
	},
	"ai-coder/securing.md": {
		Name:        "Securing agents in Coder",
		Description: "Learn how to secure agents with boundaries",
		Stage:       FeatureStageEarlyAccess,
	},
	
	// Beta features
	"ai-coder/index.md": {
		Name:        "Run AI Coding Agents in Coder",
		Description: "Learn how to run and integrate AI coding agents like GPT-Code, OpenDevin, or SWE-Agent in Coder workspaces to boost developer productivity.",
		Stage:       FeatureStageBeta,
		Experiment:  ExperimentAgenticChat,
	},
	"user-guides/desktop/index.md": {
		Name:        "Coder Desktop",
		Description: "Use Coder Desktop to access your workspace like it's a local machine",
		Stage:       FeatureStageBeta,
	},
	"admin/templates/extending-templates/prebuilt-workspaces.md": {
		Name:        "Prebuilt workspaces",
		Description: "Pre-provision a ready-to-deploy workspace with a defined set of parameters",
		Stage:       FeatureStageBeta,
		Experiment:  ExperimentWorkspacePrebuilds,
	},
	"ai-coder/create-template.md": {
		Name:        "Create a Coder template for agents",
		Description: "Create a purpose-built template for your AI agents",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/issue-tracker.md": {
		Name:        "Integrate with your issue tracker",
		Description: "Assign tickets to AI agents and interact via code reviews",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/best-practices.md": {
		Name:        "Model Context Protocols (MCP) and adding AI tools",
		Description: "Improve results by adding tools to your AI agents",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/coder-dashboard.md": {
		Name:        "Supervise agents via Coder UI",
		Description: "Interact with agents via the Coder UI",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/ide-integration.md": {
		Name:        "Supervise agents via the IDE",
		Description: "Interact with agents via VS Code or Cursor",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/headless.md": {
		Name:        "Programmatically manage agents",
		Description: "Manage agents via MCP, the Coder CLI, and/or REST API",
		Stage:       FeatureStageBeta,
	},
	"ai-coder/custom-agents.md": {
		Name:        "Custom agents",
		Description: "Learn how to use custom agents with Coder",
		Stage:       FeatureStageBeta,
	},
}

// GetFeaturesByStage returns all features with the given stage
func GetFeaturesByStage(stage FeatureStage) []Feature {
	var stageFeatures []Feature
	for _, feature := range FeatureRegistry {
		if feature.Stage == stage {
			stageFeatures = append(stageFeatures, feature)
		}
	}
	return stageFeatures
}

// GetFeatureStage returns the stage of a feature by its docs path
func GetFeatureStage(docsPath string) (FeatureStage, bool) {
	feature, ok := FeatureRegistry[docsPath]
	if !ok {
		return "", false
	}
	return feature.Stage, true
}
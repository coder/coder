import type {
	AuthorizationResponse,
	Foundation,
	ProvisionerJobLog,
	TemplateResourceRevision,
	TemplateResourceRun,
} from "#/api/typesGenerated";
import { type FileTree, traverse } from "#/utils/filetree";
import { TarWriter } from "#/utils/tar";
import { MockProvisionerJob, MockTemplate } from "./entities";

/** Independent resource permissions for Foundation management stories. */
export const MockFoundationPermissions: AuthorizationResponse = {
	canReadFoundation: true,
	canCreateFoundation: true,
	canUpdateFoundation: true,
	canUseFoundation: true,
	canDeleteFoundation: true,
};

/** Full root with an embedded local module, rather than only a main.tf preview. */
export const MockTemplateResourceFiles: FileTree = {
	"main.tf":
		'module "coder_network_policy" {\n  source = "./modules/coder-network-policy"\n}\n',
	modules: {
		"coder-network-policy": {
			"main.tf": 'resource "terraform_data" "policy" {}\n',
			"variables.tf": 'variable "template_id" { type = string }\n',
		},
	},
	"README.md": "Template-owned infrastructure\n",
};

/** Builds the same multi-file fixture for API mocks in unit tests and stories. */
export const createMockTemplateResourceArchive = async () => {
	const writer = new TarWriter();
	traverse(MockTemplateResourceFiles, (content, _name, path) => {
		if (typeof content === "string") writer.addFile(path, content);
		else writer.addFolder(path);
	});
	await writer.write();
	return writer.buffer;
};

/** Provider-independent Terraform root for immutable-source tests. */
export const MockTemplateResourceRevision: TemplateResourceRevision = {
	id: "80000000-0000-4000-8000-000000000001",
	file_id: "81000000-0000-4000-8000-000000000001",
	foundation_id: "70000000-0000-4000-8000-000000000001",
	created_at: "2026-10-21T12:00:00Z",
	source: `terraform {
  required_version = ">= 1.4.0"
}

resource "terraform_data" "example" {
  input = "Template-owned infrastructure"
}

output "example" {
  value = terraform_data.example.output
}
`,
};

/** Value-free preview with a potentially breaking consumed output change. */
export const MockFoundationPlanPreview: NonNullable<
	TemplateResourceRun["preview"]
> = {
	available: true,
	output_changes: [
		{
			name: "network_id",
			actions: ["delete"],
			unknown: false,
			sensitive: false,
		},
		{ name: "password", actions: ["update"], unknown: true, sensitive: true },
	],
	consumer_references: [
		{
			template_version_id: "60000000-0000-4000-8000-000000000001",
			output_names: ["network_id"],
			unresolved: true,
		},
	],
	advisory:
		"This preview is advisory, not a safety guarantee. Unchanged outputs do not guarantee workspace compatibility.",
};

/** Successful apply retaining its immutable revision and provisioner job. */
export const MockTemplateResourceRun: TemplateResourceRun = {
	id: "90000000-0000-4000-8000-000000000001",
	revision_id: MockTemplateResourceRevision.id,
	action: "apply",
	created_at: "2026-10-21T12:01:00Z",
	job: { ...MockProvisionerJob, status: "succeeded" },
};

/** Applied state with matching source, history and non-sensitive outputs. */
export const MockFoundation: Foundation = {
	id: "70000000-0000-4000-8000-000000000001",
	organization_id: MockTemplate.organization_id,
	name: "development-network",
	provisioner_tags: {},
	prevent_destroy_with_workspaces: true,
	consumers: { total: 0, running: 0, stopped: 0, unknown: 0 },
	output_metadata: { example: { type: "string", sensitive: false } },
	output_snapshot_id: "50000000-0000-4000-8000-000000000001",
	template_id: MockTemplate.id,
	status: "applied",
	current_revision_id: MockTemplateResourceRevision.id,
	applied_revision_id: MockTemplateResourceRevision.id,
	checkpoint_revision_id: MockTemplateResourceRevision.id,
	revisions: [MockTemplateResourceRevision],
	runs: [MockTemplateResourceRun],
	resources_available: true,
	resources: [
		{
			address: "module.coder_network_policy.terraform_data.policy",
			type: "terraform_data",
			name: "policy",
			instances: 1,
		},
	],
	outputs: { example: "Template-owned infrastructure" },
};

/** Mixed Terraform output types with sensitive values omitted by the API. */
export const MockFoundationWithOutputs: Foundation = {
	...MockFoundation,
	outputs: {
		endpoint: "https://example.com",
		enabled: false,
		replicas: 0,
		optional: null,
		zones: ["us-east-1a", "us-east-1b"],
		settings: { retries: 3, labels: { env: "development" } },
	},
	output_metadata: {
		endpoint: { type: "string", sensitive: false },
		enabled: { type: "bool", sensitive: false },
		replicas: { type: "number", sensitive: false },
		optional: { type: "string", sensitive: false },
		zones: { type: ["list", "string"], sensitive: false },
		settings: {
			type: ["object", { retries: "number", labels: ["map", "string"] }],
			sensitive: false,
		},
		password: { type: "string", sensitive: true },
		credentials: { type: ["map", "string"], sensitive: true },
	},
};

/** Persisted completion output available independently of a live job. */
export const MockTemplateResourceLog: ProvisionerJobLog = {
	id: 1,
	created_at: "2026-10-21T12:02:00Z",
	log_level: "info",
	log_source: "provisioner",
	stage: "Applying Terraform",
	output: "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.",
};

import type {
	ProvisionerJobLog,
	TemplateResourceRevision,
	TemplateResourceRun,
	TemplateResources,
} from "#/api/typesGenerated";
import { type FileTree, traverse } from "#/utils/filetree";
import { TarWriter } from "#/utils/tar";
import { MockProvisionerJob, MockTemplate } from "./entities";

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
	template_id: MockTemplate.id,
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

/** Successful apply retaining its immutable revision and provisioner job. */
export const MockTemplateResourceRun: TemplateResourceRun = {
	id: "90000000-0000-4000-8000-000000000001",
	revision_id: MockTemplateResourceRevision.id,
	action: "apply",
	created_at: "2026-10-21T12:01:00Z",
	job: { ...MockProvisionerJob, status: "succeeded" },
};

/** Applied state with matching source, history and non-sensitive outputs. */
export const MockTemplateResources: TemplateResources = {
	template_id: MockTemplate.id,
	status: "applied",
	current_revision_id: MockTemplateResourceRevision.id,
	applied_revision_id: MockTemplateResourceRevision.id,
	revisions: [MockTemplateResourceRevision],
	runs: [MockTemplateResourceRun],
	outputs: { example: "Template-owned infrastructure" },
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

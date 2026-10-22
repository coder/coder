import type {
	NetworkPolicy,
	NetworkPolicyRevision,
	NetworkPolicyRun,
	NetworkPolicyTarget,
} from "#/api/typesGenerated";
import {
	MockOrganization,
	MockTemplate,
	MockUserOwner,
	MockWorkspaceBuildLogs,
} from "./entities";

/** Persisted provisioner output suitable for both initial and cached-log states. */
export const MockNetworkPolicyLogs = MockWorkspaceBuildLogs.slice(0, 2).map(
	(log) => ({ ...log, output: "Refreshing managed network policy state..." }),
);

/** An administrable template distinct from the policy's initially attached template. */
export const MockNetworkPolicyTemplate = {
	...MockTemplate,
	id: "40000000-0000-4000-8000-000000000001",
	name: "second-template",
	display_name: "Second template",
};

/** Immutable saved HCL whose policy identity is shared by the target and run fixtures. */
export const MockNetworkPolicyRevision: NetworkPolicyRevision = {
	id: "80000000-0000-4000-8000-000000000001",
	policy_id: "70000000-0000-4000-8000-000000000001",
	hcl: 'module "coder_network_policy" {\n  source = "./module"\n}\n',
	created_at: "2026-10-20T12:00:00Z",
	created_by: MockUserOwner.id,
};

/** A successfully applied target with no drift, linked to the saved revision. */
export const MockNetworkPolicyTarget: NetworkPolicyTarget = {
	id: "90000000-0000-4000-8000-000000000001",
	policy_id: MockNetworkPolicyRevision.policy_id,
	name: "Development",
	cluster: "development-cluster",
	context: "development-context",
	namespace: "workspaces",
	provisioner_tags: { region: "eu-west-1" },
	desired_revision_id: MockNetworkPolicyRevision.id,
	applied_revision_id: MockNetworkPolicyRevision.id,
	status: "succeeded",
	drifted: false,
	last_job_id: "60000000-0000-4000-8000-000000000001",
	last_error: "",
	destroyed_at: null,
	created_at: "2026-10-20T12:00:00Z",
	updated_at: "2026-10-20T12:05:00Z",
};

/** A completed, no-change plan for the shared target and saved revision. */
export const MockNetworkPolicyRun: NetworkPolicyRun = {
	id: "50000000-0000-4000-8000-000000000001",
	policy_id: MockNetworkPolicyRevision.policy_id,
	target_id: MockNetworkPolicyTarget.id,
	revision_id: MockNetworkPolicyRevision.id,
	job_id: MockNetworkPolicyTarget.last_job_id ?? "",
	operation: "plan",
	status: "succeeded",
	error: "",
	plan: "No changes. Your infrastructure matches the configuration.",
	created_at: "2026-10-20T12:00:00Z",
	completed_at: "2026-10-20T12:05:00Z",
};

/** An active policy with linked revision, target, run, and template identities. */
export const MockNetworkPolicy: NetworkPolicy = {
	id: MockNetworkPolicyRevision.policy_id,
	organization_id: MockOrganization.id,
	name: "Developer egress",
	description: "Restrict workspace network access.",
	generation: 3,
	deletion_requested_at: null,
	deleted_at: null,
	created_at: "2026-10-20T12:00:00Z",
	updated_at: "2026-10-20T12:05:00Z",
	revisions: [MockNetworkPolicyRevision],
	template_ids: [MockTemplate.id],
	targets: [MockNetworkPolicyTarget],
	runs: [MockNetworkPolicyRun],
};

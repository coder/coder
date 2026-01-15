import type { WorkspaceAgentDevcontainer } from "api/typesGenerated";

/**
 * Returns true if this devcontainer has resources defined in Terraform.
 */
export function isTerraformDefined(
	devcontainer: WorkspaceAgentDevcontainer,
): boolean {
	return Boolean(devcontainer.subagent_id);
}

import { ExternalLinkIcon } from "lucide-react";
import type React from "react";
import { Link } from "react-router";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, type ToolStatus } from "./utils";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

/**
 * Rendering for `create_workspace` tool calls.
 *
 * Shows "Creating workspace…" while running with streaming build logs,
 * and "Created <name>" when complete with a link to view the workspace.
 * Build logs are available in a collapsible section.
 */
export const CreateWorkspaceTool: React.FC<{
	workspaceName: string;
	resultJson: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	buildId?: string;
	created?: boolean;
	labelOverride?: string;
}> = ({
	workspaceName,
	resultJson,
	status,
	isError,
	errorMessage,
	buildId,
	created = true,
	labelOverride,
}) => {
	const isRunning = status === "running";
	let rec: Record<string, unknown> | null = null;
	if (resultJson) {
		try {
			const parsed = JSON.parse(resultJson);
			rec = asRecord(parsed);
		} catch {
			rec = asRecord(resultJson);
		}
	}
	const ownerName = rec ? asString(rec.owner_name) : "";
	const wsName = rec ? asString(rec.workspace_name) : workspaceName;
	const workspaceLink = ownerName && wsName ? `/@${ownerName}/${wsName}` : null;

	const label = isRunning
		? "Creating workspace…"
		: labelOverride
			? labelOverride
			: isError
				? `Failed to create ${wsName || "workspace"}`
				: created === false
					? `Workspace ${wsName} already exists`
					: wsName
						? `Created ${wsName}`
						: "Created workspace";

	const hasBuildLogs = isRunning || Boolean(buildId);

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to create workspace"}
			hasContent={hasBuildLogs}
			defaultExpanded={isRunning}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<ToolCall.LeadingIcon name="create_workspace" />
					<ToolCall.Label>{label}</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{workspaceLink && !isRunning && (
					<ToolCall.HeaderActions>
						<Link
							to={workspaceLink}
							className="inline-flex align-middle text-content-secondary opacity-50 transition-opacity hover:opacity-100"
							aria-label="View workspace"
						>
							<ExternalLinkIcon className="size-3" />
						</Link>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

import { ExternalLinkIcon } from "lucide-react";
import type React from "react";
import { Link } from "react-router";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { ToolCall } from "./ToolCall";
import { asString, parseArgs, type ToolStatus } from "./utils";
import { WorkspaceAgentLogSection } from "./WorkspaceAgentLogSection";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";
import type { WorkspaceToolOutcome } from "./workspaceToolOutcome";
import { useWorkspaceToolStage } from "./workspaceToolStage";

/**
 * Rendering for `create_workspace` tool calls.
 *
 * The collapsed row shows the current build or agent stage while
 * running, and "Created <name>" when complete with a link to view the
 * workspace. Build and agent logs are in the expandable section.
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
	/** Agent wait outcome of a successful create call. */
	outcome?: WorkspaceToolOutcome;
}> = ({
	workspaceName,
	resultJson,
	status,
	isError,
	errorMessage,
	buildId,
	created = true,
	labelOverride,
	outcome,
}) => {
	const isRunning = status === "running";
	const stage = useWorkspaceToolStage("create", isRunning);
	const rec = parseArgs(resultJson);
	const ownerName = rec ? asString(rec.owner_name) : "";
	const wsName = rec ? asString(rec.workspace_name) : workspaceName;
	const workspaceLink =
		ownerName && wsName && !isRunning ? `/@${ownerName}/${wsName}` : null;

	let success = "Created workspace";
	if (created === false) {
		success = `Workspace ${wsName} already exists`;
	} else if (wsName) {
		success = `Created ${wsName}`;
	}

	const failure = outcome && "failure" in outcome ? outcome.failure : undefined;
	const notice = outcome && "notice" in outcome ? outcome.notice : undefined;

	let label: string;
	if (isRunning) {
		label = stage ?? "Creating workspace…";
	} else if (labelOverride) {
		label = labelOverride;
	} else if (isError) {
		label = `Failed to create ${wsName || "workspace"}`;
	} else if (failure) {
		label = `${success}, ${failure.labelSuffix}`;
	} else {
		label = success;
	}

	const hasBuildLogs = isRunning || Boolean(buildId);

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError || Boolean(failure)}
			errorMessage={
				failure?.tooltip || errorMessage || "Failed to create workspace"
			}
			hasContent={hasBuildLogs}
			defaultExpanded={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<ToolCall.LeadingIcon name="create_workspace" />
					<ToolCall.Label>{label}</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{(workspaceLink || notice) && (
					<ToolCall.HeaderActions>
						{notice && (
							<InfoTooltip
								type="info"
								size="small"
								ariaLabel="Startup scripts notice"
							>
								{notice}
							</InfoTooltip>
						)}
						{workspaceLink && (
							<Link
								to={workspaceLink}
								className="inline-flex align-middle text-content-secondary opacity-50 transition-opacity hover:opacity-100"
								aria-label="View workspace"
							>
								<ExternalLinkIcon className="size-3" />
							</Link>
						)}
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
				<WorkspaceAgentLogSection status={status} buildId={buildId} />
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

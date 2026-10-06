import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";
import { WorkspaceLogBox } from "./WorkspaceLogBox";
import type { WorkspaceToolOutcome } from "./workspaceToolOutcome";
import { useWorkspaceToolStage } from "./workspaceToolStage";

type WorkspaceLifecycleToolProps = {
	action: "start" | "stop";
	status: ToolStatus;
	buildId?: string;
	workspaceName: string;
	isError: boolean;
	errorMessage?: string;
	noBuild?: boolean;
	labelOverride?: string;
	/** Agent wait outcome of a successful start call. */
	outcome?: WorkspaceToolOutcome;
};

export const WorkspaceLifecycleTool: React.FC<WorkspaceLifecycleToolProps> = ({
	action,
	status,
	buildId,
	workspaceName,
	isError,
	errorMessage,
	noBuild,
	labelOverride,
	outcome,
}) => {
	const isRunning = status === "running";
	const stage = useWorkspaceToolStage(action, isRunning);
	const success = `${action === "start" ? "Started" : "Stopped"} ${workspaceName || "workspace"}`;

	const failure = outcome && "failure" in outcome ? outcome.failure : undefined;
	const notice = outcome && "notice" in outcome ? outcome.notice : undefined;

	let label: string;
	if (isRunning) {
		label =
			stage ??
			(action === "start" ? "Starting workspace…" : "Stopping workspace…");
	} else if (labelOverride) {
		label = labelOverride;
	} else if (isError) {
		label = `Failed to ${action} ${workspaceName || "workspace"}`;
	} else if (failure) {
		label = `${success}, ${failure.labelSuffix}`;
	} else {
		label = success;
	}

	const hasBuildLogs = (isRunning || Boolean(buildId)) && !noBuild;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError || Boolean(failure)}
			errorMessage={
				failure?.tooltip || errorMessage || `Failed to ${action} workspace`
			}
			hasContent={hasBuildLogs}
			defaultExpanded={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<ToolCall.LeadingIcon name={`${action}_workspace`} />
					<ToolCall.Label>{label}</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{notice && (
					<ToolCall.HeaderActions>
						<InfoTooltip
							type="info"
							size="small"
							ariaLabel="Startup scripts notice"
						>
							{notice}
						</InfoTooltip>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				<WorkspaceLogBox
					status={status}
					buildId={buildId}
					action={action}
					notice={notice}
				/>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

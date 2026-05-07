import {
	ExternalLinkIcon,
	LoaderIcon,
	MonitorIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type React from "react";
import { Link } from "react-router";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ToolCollapsible } from "./ToolCollapsible";
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
}> = ({
	workspaceName,
	resultJson,
	status,
	isError,
	errorMessage,
	buildId,
	created = true,
}) => {
	const isRunning = status === "running";
	let rec: Record<string, unknown> | null = null;
	if (resultJson) {
		try {
			const parsed = JSON.parse(resultJson);
			rec = asRecord(parsed);
		} catch {
			// resultJson might already be an object or invalid JSON
			rec = asRecord(resultJson);
		}
	}
	const ownerName = rec ? asString(rec.owner_name) : "";
	const wsName = rec ? asString(rec.workspace_name) : workspaceName;
	const workspaceLink = ownerName && wsName ? `/@${ownerName}/${wsName}` : null;

	const label = isRunning
		? "Creating workspace…"
		: isError
			? `Failed to create ${wsName || "workspace"}`
			: created === false
				? `Workspace ${wsName} already exists`
				: wsName
					? `Created ${wsName}`
					: "Created workspace";

	const hasBuildLogs = isRunning || Boolean(buildId);

	const header = (
		<>
			<MonitorIcon className="h-4 w-4 shrink-0 text-current" />
			<span className="text-[13px]">{label}</span>
			{isError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-current" />
					</TooltipTrigger>
					<TooltipContent>
						{errorMessage || "Failed to create workspace"}
					</TooltipContent>
				</Tooltip>
			)}
			{isRunning && (
				<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
			)}
			{workspaceLink && !isRunning && (
				<Link
					to={workspaceLink}
					onClick={(e) => e.stopPropagation()}
					className="ml-1 inline-flex align-middle text-content-secondary opacity-50 transition-opacity hover:opacity-100"
					aria-label="View workspace"
				>
					<ExternalLinkIcon className="h-3 w-3" />
				</Link>
			)}
		</>
	);

	return (
		<div className="w-full">
			<ToolCollapsible
				header={header}
				hasContent={hasBuildLogs}
				defaultExpanded={isRunning}
			>
				<WorkspaceBuildLogSection status={status} buildId={buildId} />
			</ToolCollapsible>
		</div>
	);
};

import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, ExternalLinkIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import { asRecord, asString, type ToolStatus } from "./utils";

/**
 * Rendering for `create_workspace` tool calls.
 *
 * Shows "Creating workspace…" while running, and "Created <name>" when
 * complete with a link to view the workspace.
 */
export const CreateWorkspaceTool: React.FC<{
	workspaceName: string;
	resultJson: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ workspaceName, resultJson, status, isError, errorMessage }) => {
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
		: wsName
			? `Created ${wsName}`
			: "Created workspace";

	return (
		<div className="w-full">
			<div className="flex items-center gap-2">
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to create workspace"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
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
			</div>
		</div>
	);
};

import { MonitorIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type { Workspace } from "#/api/typesGenerated";
import {
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
} from "#/utils/workspace";

export const statusVariantMap: Record<
	DisplayWorkspaceStatusType,
	StatusIndicatorProps["variant"]
> = {
	active: "pending",
	inactive: "inactive",
	success: "success",
	error: "failed",
	danger: "warning",
	warning: "warning",
};

interface WorkspaceBadgeProps {
	workspace: Workspace;
	route: string;
}

export const WorkspaceBadge: FC<WorkspaceBadgeProps> = ({
	workspace,
	route,
}) => {
	const { text, type } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);
	const variant = statusVariantMap[workspace.health.healthy ? type : "warning"];

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					to={route}
					target="_blank"
					className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary"
				>
					<MonitorIcon className="size-3.5 shrink-0" />
					<span className="hidden truncate max-w-[120px] md:inline">
						{workspace.name}
					</span>
					<StatusIndicatorDot variant={variant} size="sm" />
				</Link>
			</TooltipTrigger>
			<TooltipContent>
				{workspace.name} – {text}
			</TooltipContent>
		</Tooltip>
	);
};

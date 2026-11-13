import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { BuildIcon } from "#/modules/workspaces/BuildIcon/BuildIcon";
import { createDayString } from "#/utils/createDayString";
import {
	getBuildReasonLabel,
	getDisplayWorkspaceBuildInitiatedBy,
	systemBuildReasons,
} from "#/utils/workspace";

const getDisplayBuildReason = (build: WorkspaceBuild) =>
	!systemBuildReasons.includes(build.reason) && build.transition === "start"
		? getBuildReasonLabel(build.reason)
		: undefined;

type WorkspaceBuildReasonTooltipProps = {
	build: WorkspaceBuild;
	children: React.ReactElement;
};

/**
 * Shows the build reason when its trigger, the build's link, is hovered or
 * focused. Renders the trigger alone for builds without a displayed reason.
 */
export const WorkspaceBuildReasonTooltip: React.FC<
	WorkspaceBuildReasonTooltipProps
> = ({ build, children }) => {
	const buildReason = getDisplayBuildReason(build);
	if (!buildReason) {
		return children;
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>{children}</TooltipTrigger>
			<TooltipContent side="right">{buildReason}</TooltipContent>
		</Tooltip>
	);
};

type WorkspaceBuildDataProps = {
	build: WorkspaceBuild;
};

export const WorkspaceBuildData: React.FC<WorkspaceBuildDataProps> = ({
	build,
}) => {
	const buildReason = getDisplayBuildReason(build);

	return (
		<div className="flex items-center gap-3 leading-normal">
			<BuildIcon transition={build.transition} jobStatus={build.job.status} />
			<div className="overflow-hidden flex flex-col">
				<div
					className={cn(
						"text-content-primary text-ellipsis overflow-hidden",
						"whitespace-nowrap flex items-center gap-1",
					)}
				>
					<span className="capitalize">{build.transition}</span> by{" "}
					<span className="font-medium">
						{getDisplayWorkspaceBuildInitiatedBy(build)}
					</span>
					{buildReason && (
						// This renders inside the build's link, so the reason joins the
						// link's name and the link triggers the tooltip, avoiding a
						// nested focusable control.
						<>
							<InfoIcon
								aria-hidden
								className="text-content-secondary size-icon-xs -mt-px"
							/>
							<span className="sr-only">, build reason: {buildReason}</span>
						</>
					)}
				</div>
				<div className="text-xs font-normal text-content-secondary">
					{createDayString(build.created_at)}
				</div>
			</div>
		</div>
	);
};

export const WorkspaceBuildDataSkeleton = () => {
	return (
		<div className="flex items-center gap-3 leading-normal">
			<Skeleton variant="circular" width={16} height={16} />
			<div className="flex flex-col">
				<Skeleton variant="text" width={94} height={10} />
				<Skeleton variant="text" width={60} height={8} />
			</div>
		</div>
	);
};

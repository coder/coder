import { useTheme } from "@emotion/react";
import { InfoIcon } from "lucide-react";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { BuildIcon } from "#/components/BuildIcon/BuildIcon";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { createDayString } from "#/utils/createDayString";
import {
	buildReasonLabels,
	getDisplayWorkspaceBuildInitiatedBy,
	getDisplayWorkspaceBuildStatus,
	systemBuildReasons,
} from "#/utils/workspace";

export const WorkspaceBuildData = ({ build }: { build: WorkspaceBuild }) => {
	const theme = useTheme();
	const statusType = getDisplayWorkspaceBuildStatus(theme, build).type;

	return (
		<div className="flex flex-row items-center gap-3 leading-normal">
			<BuildIcon
				transition={build.transition}
				className="size-4"
				css={{
					color: theme.roles[statusType].fill.solid,
				}}
			/>
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
					{!systemBuildReasons.includes(build.reason) &&
						build.transition === "start" && (
							<Tooltip>
								<TooltipTrigger asChild>
									<InfoIcon
										css={(theme) => ({
											color: theme.palette.info.light,
										})}
										className="size-icon-xs -mt-px"
									/>
								</TooltipTrigger>
								<TooltipContent side="bottom">
									{buildReasonLabels[build.reason]}
								</TooltipContent>
							</Tooltip>
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
		<div className="flex flex-row items-center gap-3 leading-normal">
			<Skeleton variant="circular" width={16} height={16} />
			<div className="flex flex-col">
				<Skeleton variant="text" width={94} height={10} />
				<Skeleton variant="text" width={60} height={8} />
			</div>
		</div>
	);
};

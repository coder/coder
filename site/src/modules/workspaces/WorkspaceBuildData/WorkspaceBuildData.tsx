import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import type { WorkspaceBuild } from "api/typesGenerated";
import { BuildIcon } from "components/BuildIcon/BuildIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";
import { createDayString } from "utils/createDayString";
import {
	buildReasonLabels,
	getDisplayWorkspaceBuildInitiatedBy,
	getDisplayWorkspaceBuildStatus,
	systemBuildReasons,
} from "utils/workspace";

export const WorkspaceBuildData = ({ build }: { build: WorkspaceBuild }) => {
	const theme = useTheme();
	const statusType = getDisplayWorkspaceBuildStatus(theme, build).type;

	return (
		<div css={styles.root}>
			<BuildIcon
				transition={build.transition}
				css={{
					color: theme.roles[statusType].fill.solid,
				}}
				className="size-4"
			/>
			<div className="overflow-hidden">
				<div
					css={{
						color: theme.palette.text.primary,
					}}
					className="truncate flex items-center gap-1"
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
				<div
					css={{
						color: theme.palette.text.secondary,
					}}
					className="text-sm leading-none"
				>
					{createDayString(build.created_at)}
				</div>
			</div>
		</div>
	);
};

export const WorkspaceBuildDataSkeleton = () => {
	return (
		<div css={styles.root}>
			<Skeleton variant="circular" width={16} height={16} />
			<div>
				<Skeleton variant="text" width={94} height={16} />
				<Skeleton variant="text" width={60} height={14} className="mt-0.5" />
			</div>
		</div>
	);
};

const styles = {
	root: {
		display: "flex",
		flexDirection: "row",
		alignItems: "center",
		gap: 12,
		lineHeight: "1.5",
	},
} satisfies Record<string, Interpolation<Theme>>;

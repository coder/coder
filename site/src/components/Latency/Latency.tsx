import CircularProgress from "@mui/material/CircularProgress";
import { Abbr } from "components/Abbr/Abbr";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleHelpIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";
import { getLatencyColor } from "utils/latency";

interface LatencyProps {
	latency?: number;
	isLoading?: boolean;
	className?: string;
}

export const Latency: FC<LatencyProps> = ({
	latency,
	isLoading,
	className,
}) => {
	// Always use the no latency color for loading.
	const latencyColor = getLatencyColor(isLoading ? undefined : latency);

	if (isLoading) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					{/**
					 * Spinning progress icon must be placed inside a fixed-size container,
					 * to ensure tooltip remains stationary when opened
					 */}
					<div
						className={cn(
							"size-4 flex flex-wrap place-content-center",
							className,
						)}
					>
						<CircularProgress
							className="!size-icon-xs"
							style={{ color: latencyColor }}
						/>
					</div>
				</TooltipTrigger>
				<TooltipContent side="bottom">Loading latency...</TooltipContent>
			</Tooltip>
		);
	}

	if (!latency) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<CircleHelpIcon
						aria-label="Latency not available"
						className={cn("!size-icon-sm", latencyColor, className)}
					/>
				</TooltipTrigger>
				<TooltipContent side="bottom">Latency not available</TooltipContent>
			</Tooltip>
		);
	}

	return (
		<div className={cn("text-sm", latencyColor, className)}>
			<span className="sr-only">Latency: </span>
			{latency.toFixed(0)}
			<Abbr title="milliseconds">ms</Abbr>
		</div>
	);
};

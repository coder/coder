import { useTheme } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import { Abbr } from "components/Abbr/Abbr";
import MiniTooltip from "components/MiniTooltip/MiniTooltip";
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
	const theme = useTheme();
	// Always use the no latency color for loading.
	const color = getLatencyColor(theme, isLoading ? undefined : latency);

	if (isLoading) {
		return (
			<MiniTooltip title="Loading latency...">
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
					<CircularProgress className="!size-icon-xs" style={{ color }} />
				</div>
			</MiniTooltip>
		);
	}

	if (!latency) {
		return (
			<MiniTooltip title="Latency not available">
				<CircleHelpIcon
					className={cn("!size-icon-sm", className)}
					style={{ color }}
				/>
			</MiniTooltip>
		);
	}

	return (
		<div className={cn("text-sm", className)} style={{ color }}>
			<span className="sr-only">Latency: </span>
			{latency.toFixed(0)}
			<Abbr title="milliseconds">ms</Abbr>
		</div>
	);
};

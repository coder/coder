import { useTheme } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import Tooltip from "@mui/material/Tooltip";
import { Abbr } from "components/Abbr/Abbr";
import { CircleHelpIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";
import { getLatencyColor } from "utils/latency";

interface LatencyProps {
	latency?: number;
	isLoading?: boolean;
	className?: string;
	iconClassName?: string;
}

export const Latency: FC<LatencyProps> = ({
	latency,
	isLoading,
	className,
	iconClassName,
}) => {
	const theme = useTheme();
	// Always use the no latency color for loading.
	const color = getLatencyColor(theme, isLoading ? undefined : latency);

	if (isLoading) {
		return (
			<Tooltip title="Loading latency..." className={className}>
				<CircularProgress
					className={cn("!size-icon-xs", iconClassName)}
					style={{ color }}
				/>
			</Tooltip>
		);
	}

	if (!latency) {
		return (
			<Tooltip title="Latency not available" className={className}>
				<CircleHelpIcon
					className={cn("!size-icon-sm", iconClassName)}
					style={{ color }}
				/>
			</Tooltip>
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

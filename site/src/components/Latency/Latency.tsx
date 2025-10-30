import { useTheme } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import { TooltipProvider } from "@radix-ui/react-tooltip";
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
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<div className="!size-icon-xs">
							<CircularProgress
								className={cn("!size-icon-xs", iconClassName)}
								style={{ color }}
							/>
						</div>
					</TooltipTrigger>
					<TooltipContent>Loading latency...</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	if (!latency) {
		return (
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<CircleHelpIcon
							className={cn("!size-icon-sm", iconClassName)}
							style={{ color }}
						/>
					</TooltipTrigger>
					<TooltipContent>Latency not available</TooltipContent>
				</Tooltip>
			</TooltipProvider>
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

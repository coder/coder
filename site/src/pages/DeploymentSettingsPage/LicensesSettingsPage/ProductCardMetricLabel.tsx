import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

/**
 * Metric label with an info tooltip, shared by the product cards on the
 * license card's Products grid.
 */
export const ProductCardMetricLabel: FC<{ label: string; tooltip: string }> = ({
	label,
	tooltip,
}) => (
	<div className="flex items-center gap-1 font-medium text-content-secondary">
		<span>{label}</span>
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={`${label} information`}
					className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
				>
					<InfoIcon className="size-3" />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs">
				{tooltip}
			</TooltipContent>
		</Tooltip>
	</div>
);

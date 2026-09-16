import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeModelIcon } from "./icons/AIBridgeModelIcon";

/**
 * Names a single model with its icon, or counts them when there are several
 * and lists them on hover or focus. Renders nothing for an empty list.
 */
export const ModelsBadge: FC<{ models: readonly string[] }> = ({ models }) => {
	if (models.length === 0) {
		return null;
	}
	if (models.length > 1) {
		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge asChild hover className="max-w-full">
							<button
								type="button"
								onClick={(event) => event.stopPropagation()}
							>
								{models.length} models
							</button>
						</Badge>
					</TooltipTrigger>
					<TooltipContent side="top" align="start">
						<ul className="m-0 flex list-none flex-col gap-1 p-0">
							{models.map((model) => (
								<li key={model} className="flex items-center gap-1.5">
									<AIBridgeModelIcon model={model} className="size-icon-xs" />
									{model}
								</li>
							))}
						</ul>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	return (
		<Badge className="gap-1.5 max-w-full">
			<div className="shrink-0 flex items-center">
				<AIBridgeModelIcon model={models[0]} className="size-icon-xs" />
			</div>
			<span className="truncate min-w-0">{models[0]}</span>
		</Badge>
	);
};

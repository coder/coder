import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

/**
 * Names a single provider with its icon, or counts them when there are
 * several and lists them on hover or focus. Renders nothing for an empty list.
 */
export const ProvidersBadge: FC<{ providers: readonly string[] }> = ({
	providers,
}) => {
	if (providers.length === 0) {
		return null;
	}
	if (providers.length > 1) {
		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge asChild hover className="max-w-full">
							<button
								type="button"
								onClick={(event) => event.stopPropagation()}
							>
								{providers.length} providers
							</button>
						</Badge>
					</TooltipTrigger>
					<TooltipContent side="top" align="start">
						<ul className="m-0 flex list-none flex-col gap-1 p-0">
							{providers.map((provider) => (
								<li key={provider} className="flex items-center gap-1.5">
									<AIBridgeProviderIcon
										provider={provider}
										className="size-icon-xs"
									/>
									{getProviderDisplayName(provider)}
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
				<AIBridgeProviderIcon
					provider={providers[0]}
					className="size-icon-xs"
				/>
			</div>
			<span className="truncate min-w-0">
				{getProviderDisplayName(providers[0])}
			</span>
		</Badge>
	);
};

import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

/**
 * Names a single client with its icon, or counts them when there are several
 * and lists them on hover or focus. A missing client is reported as Unknown.
 * Renders nothing for an empty list.
 */
export const ClientsBadge: FC<{ clients: readonly (string | null)[] }> = ({
	clients,
}) => {
	if (clients.length === 0) {
		return null;
	}
	if (clients.length > 1) {
		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge asChild hover className="max-w-full">
							<button
								type="button"
								onClick={(event) => event.stopPropagation()}
							>
								{clients.length} clients
							</button>
						</Badge>
					</TooltipTrigger>
					<TooltipContent side="top" align="start">
						<ul className="m-0 flex list-none flex-col gap-1 p-0">
							{clients.map((client) => (
								<li
									key={client ?? "Unknown"}
									className="flex items-center gap-1.5"
								>
									<AIBridgeClientIcon
										client={client}
										className="size-icon-xs"
									/>
									{client ?? "Unknown"}
								</li>
							))}
						</ul>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	const client = clients[0];
	return (
		<Badge className="gap-1.5 max-w-full">
			<div className="shrink-0 flex items-center">
				<AIBridgeClientIcon client={client} className="size-icon-xs" />
			</div>
			<span className="truncate min-w-0">{client ?? "Unknown"}</span>
		</Badge>
	);
};

import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

/**
 * Names a single client with its icon, or counts them when there are several.
 * A missing client is reported as Unknown. Renders nothing for an empty list.
 */
export const ClientsBadge: FC<{ clients: readonly (string | null)[] }> = ({
	clients,
}) => {
	if (clients.length === 0) {
		return null;
	}
	if (clients.length > 1) {
		return <Badge className="max-w-full">{clients.length} clients</Badge>;
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

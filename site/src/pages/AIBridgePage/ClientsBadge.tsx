import type { FC } from "react";
import { DimensionBadge } from "./DimensionBadge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

// A missing client is reported as Unknown, like the sessions list.
export const ClientsBadge: FC<{ clients: readonly (string | null)[] }> = ({
	clients,
}) => (
	<DimensionBadge
		noun="clients"
		items={clients.map((client) => ({
			key: client ?? "Unknown",
			label: client ?? "Unknown",
			icon: <AIBridgeClientIcon client={client} className="size-icon-xs" />,
		}))}
	/>
);

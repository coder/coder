import type { FC } from "react";
import { DimensionBadge } from "./DimensionBadge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

type ClientsBadgeProps = {
	clients: readonly string[];
};

export const ClientsBadge: FC<ClientsBadgeProps> = ({ clients }) => (
	<DimensionBadge
		noun="clients"
		items={clients.map((client) => ({
			key: client,
			label: client,
			icon: <AIBridgeClientIcon client={client} className="size-icon-xs" />,
		}))}
	/>
);

import type { FC } from "react";
import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

type ClientsBadgeProps = {
	clients: readonly string[];
};

export const ClientsBadge: FC<ClientsBadgeProps> = ({ clients }) => (
	<ItemsBadge
		noun="clients"
		items={clients.map((client) => ({
			key: client,
			label: client,
			icon: <AIBridgeClientIcon client={client} className="size-icon-xs" />,
		}))}
	/>
);

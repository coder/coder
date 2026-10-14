import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";

type ClientsBadgeProps = {
	clients: readonly string[];
};

export const ClientsBadge: React.FC<ClientsBadgeProps> = ({ clients }) => (
	<ItemsBadge
		noun="clients"
		items={clients.map((client) => ({
			key: client,
			label: client,
			icon: <AIBridgeClientIcon client={client} className="size-icon-xs" />,
		}))}
	/>
);

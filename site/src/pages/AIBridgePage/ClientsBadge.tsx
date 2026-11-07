import { TooltipMessage, TooltipTitle } from "#/components/Tooltip/Tooltip";
import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeClientIcon } from "./icons/AIBridgeClientIcon";
import { UNKNOWN_CLIENT } from "./utils";

type ClientsBadgeProps = {
	clients: readonly string[];
	/**
	 * Raw request User-Agent, shown on hover when the client could not be
	 * inferred so admins can tell which tool sent the request.
	 */
	userAgent?: string;
};

export const ClientsBadge: React.FC<ClientsBadgeProps> = ({
	clients,
	userAgent,
}) => (
	<ItemsBadge
		noun="clients"
		items={clients.map((client) => ({
			key: client,
			label: client,
			icon: <AIBridgeClientIcon client={client} className="size-icon-xs" />,
			description:
				client === UNKNOWN_CLIENT && userAgent ? (
					<>
						<TooltipTitle>User agent</TooltipTitle>
						<TooltipMessage className="font-mono text-xs wrap-anywhere">
							{userAgent}
						</TooltipMessage>
					</>
				) : undefined,
		}))}
	/>
);

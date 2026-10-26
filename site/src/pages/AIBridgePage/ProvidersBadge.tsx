import type { AIBridgeProvider } from "#/api/typesGenerated";
import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

type ProvidersBadgeProps = {
	providers: readonly string[];
	/** Labels provider names; values without a match are treated as types. */
	configuredProviders?: readonly AIBridgeProvider[];
};

export const ProvidersBadge: React.FC<ProvidersBadgeProps> = ({
	providers,
	configuredProviders,
}) => (
	<ItemsBadge
		noun="providers"
		items={providers.map((provider) => {
			const configured = configuredProviders?.find((p) => p.name === provider);
			return {
				key: provider,
				label: configured
					? configured.display_name || configured.name
					: getProviderDisplayName(provider),
				icon: (
					<AIBridgeProviderIcon
						provider={configured?.type ?? provider}
						className="size-icon-xs"
					/>
				),
			};
		})}
	/>
);

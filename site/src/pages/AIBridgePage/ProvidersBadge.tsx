import type { AIBridgeProvider } from "#/api/typesGenerated";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

type ProvidersBadgeProps = {
	providers: readonly string[];
	/** Configured providers by name; matches supply the label and icon. */
	providersByName?: ReadonlyMap<string, AIBridgeProvider>;
};

export const ProvidersBadge: React.FC<ProvidersBadgeProps> = ({
	providers,
	providersByName,
}) => (
	<ItemsBadge
		noun="providers"
		items={providers.map((provider) => {
			const configured = providersByName?.get(provider);
			return {
				key: provider,
				label: configured
					? configured.display_name || configured.name
					: getProviderDisplayName(provider),
				icon: configured ? (
					<ProviderIcon
						provider={configured.type}
						icon={configured.icon}
						className="size-icon-xs"
					/>
				) : (
					<AIBridgeProviderIcon provider={provider} className="size-icon-xs" />
				),
			};
		})}
	/>
);

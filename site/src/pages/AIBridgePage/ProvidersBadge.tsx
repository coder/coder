import type { AIBridgeProvider } from "#/api/typesGenerated";
import { ItemsBadge } from "./ItemsBadge";
import { ProviderNameIcon } from "./icons/ProviderNameIcon";
import { getProviderNameLabel } from "./utils";

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
		items={providers.map((provider) => ({
			key: provider,
			label: getProviderNameLabel(provider, providersByName),
			icon: (
				<ProviderNameIcon
					provider={provider}
					providersByName={providersByName}
					className="size-icon-xs"
				/>
			),
		}))}
	/>
);

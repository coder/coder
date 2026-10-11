import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

type ProvidersBadgeProps = {
	providers: readonly string[];
};

export const ProvidersBadge: React.FC<ProvidersBadgeProps> = ({
	providers,
}) => (
	<ItemsBadge
		noun="providers"
		items={providers.map((provider) => ({
			key: provider,
			label: getProviderDisplayName(provider),
			icon: (
				<AIBridgeProviderIcon provider={provider} className="size-icon-xs" />
			),
		}))}
	/>
);

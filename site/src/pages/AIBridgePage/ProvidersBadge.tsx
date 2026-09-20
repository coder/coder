import type { FC } from "react";
import { DimensionBadge } from "./DimensionBadge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

type ProvidersBadgeProps = {
	providers: readonly string[];
};

export const ProvidersBadge: FC<ProvidersBadgeProps> = ({ providers }) => (
	<DimensionBadge
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

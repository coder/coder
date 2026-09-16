import type { FC } from "react";
import { DimensionBadge } from "./DimensionBadge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

export const ProvidersBadge: FC<{ providers: readonly string[] }> = ({
	providers,
}) => (
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

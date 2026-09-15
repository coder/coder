import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { AIBridgeProviderIcon } from "./icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "./utils";

/**
 * Names a single provider with its icon, or counts them when there are
 * several. Renders nothing for an empty list.
 */
export const ProvidersBadge: FC<{ providers: readonly string[] }> = ({
	providers,
}) => {
	if (providers.length === 0) {
		return null;
	}
	if (providers.length > 1) {
		return <Badge className="max-w-full">{providers.length} providers</Badge>;
	}
	return (
		<Badge className="gap-1.5 max-w-full">
			<div className="shrink-0 flex items-center">
				<AIBridgeProviderIcon
					provider={providers[0]}
					className="size-icon-xs"
				/>
			</div>
			<span className="truncate min-w-0">
				{getProviderDisplayName(providers[0])}
			</span>
		</Badge>
	);
};

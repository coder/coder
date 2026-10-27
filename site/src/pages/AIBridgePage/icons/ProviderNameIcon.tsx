import type { AIBridgeProvider } from "#/api/typesGenerated";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeProviderIcon } from "./AIBridgeProviderIcon";

type ProviderNameIconProps = {
	provider: string;
	/** Configured providers by name; a name with no match is treated as a type. */
	providersByName: ReadonlyMap<string, AIBridgeProvider> | undefined;
	className: string;
};

/** Uses the configured provider's icon, else the icon for the name as a type. */
export const ProviderNameIcon: React.FC<ProviderNameIconProps> = ({
	provider,
	providersByName,
	className,
}) => {
	const configured = providersByName?.get(provider);
	return configured ? (
		<ProviderIcon
			provider={configured.type}
			icon={configured.icon}
			className={className}
		/>
	) : (
		<AIBridgeProviderIcon provider={provider} className={className} />
	);
};

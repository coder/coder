import type { FC } from "react";
import { useMutation } from "react-query";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import type { PremiumFunnelSource } from "#/api/typesGenerated";
import type { PaywallProps } from "#/components/Paywall/Paywall";
import { PaywallSmall } from "#/components/Paywall/PaywallSmall";
import { trackPremiumFunnelClick } from "./premiumFunnelAttribution";

type PremiumPaywallSmallProps = Omit<PaywallProps, "onCTAClick"> & {
	source: PremiumFunnelSource;
};

/**
 * The inline premium paywall, wired to conversion telemetry. Prefer this over
 * PaywallSmall so every surface is attributable.
 */
export const PremiumPaywallSmall: FC<PremiumPaywallSmallProps> = ({
	source,
	...paywallProps
}) => {
	const { mutate: reportClick } = useMutation(reportPremiumFunnelEvent());

	return (
		<PaywallSmall
			{...paywallProps}
			onCTAClick={() => reportClick(trackPremiumFunnelClick(source, "small"))}
		/>
	);
};

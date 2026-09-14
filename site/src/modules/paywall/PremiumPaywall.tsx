import type { FC } from "react";
import { useMutation } from "react-query";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import type { PremiumFunnelSource } from "#/api/typesGenerated";
import type { PaywallProps } from "#/components/Paywall/Paywall";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { trackPremiumFunnelClick } from "./premiumFunnelAttribution";

type PremiumPaywallProps = Omit<PaywallProps, "onCTAClick"> & {
	source: PremiumFunnelSource;
};

/**
 * The full page premium paywall, wired to conversion telemetry. Prefer this
 * over PaywallPremium so every surface is attributable.
 */
export const PremiumPaywall: FC<PremiumPaywallProps> = ({
	source,
	...paywallProps
}) => {
	const { mutate: reportClick } = useMutation(reportPremiumFunnelEvent());

	return (
		<PaywallPremium
			{...paywallProps}
			onCTAClick={() => reportClick(trackPremiumFunnelClick(source, "premium"))}
		/>
	);
};

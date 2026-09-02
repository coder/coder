import type { ComponentProps, FC } from "react";
import { useMutation } from "react-query";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import type { PremiumFunnelSource } from "#/api/typesGenerated";
import { PaywallAIGovernance } from "#/components/Paywall/PaywallAIGovernance";
import { trackPremiumFunnelClick } from "./premiumFunnelAttribution";

type PremiumPaywallAIGovernanceProps = Omit<
	ComponentProps<typeof PaywallAIGovernance>,
	"onCTAClick"
> & {
	source: PremiumFunnelSource;
};

/**
 * The AI gateway paywall, wired to conversion telemetry. Prefer this over
 * PaywallAIGovernance so every surface is attributable.
 */
export const PremiumPaywallAIGovernance: FC<
	PremiumPaywallAIGovernanceProps
> = ({ source, ...paywallProps }) => {
	const { mutate: reportClick } = useMutation(reportPremiumFunnelEvent());

	return (
		<PaywallAIGovernance
			{...paywallProps}
			onCTAClick={() =>
				reportClick(trackPremiumFunnelClick(source, "ai_governance"))
			}
		/>
	);
};

import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import type { Feature } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { ProductCardMetricLabel } from "./ProductCardMetricLabel";

type AIGovernanceProductCardProps = {
	/** The merged aibridge entitlement feature (AI Gateway). */
	aibridgeFeature?: Feature;
	/** The merged boundary entitlement feature (Agent Firewall). */
	boundaryFeature?: Feature;
};

// TODO: placeholder tooltip copy pending product review.
const aiGatewayTooltip =
	"Routes your deployment's LLM traffic through Coder for auditing and cost controls.";
const agentFirewallTooltip =
	"Monitors and controls the network calls agents make from workspaces.";

export const AIGovernanceProductCard: FC<AIGovernanceProductCardProps> = ({
	aibridgeFeature,
	boundaryFeature,
}) => {
	const isGatewayEnabled = aibridgeFeature?.enabled === true;
	const isFirewallInUse = boundaryFeature?.enabled === true;

	return (
		<div className="min-w-[320px] flex-1 rounded-sm border border-solid border-border px-6 py-4">
			<div className="text-sm font-medium text-content-primary">
				AI Governance
			</div>
			<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
				<div>
					<ProductCardMetricLabel
						label="AI Gateway"
						tooltip={aiGatewayTooltip}
					/>
					<div className="mt-0.5 text-sm font-medium text-content-primary">
						{isGatewayEnabled ? "Enabled" : "Not enabled"}
					</div>
				</div>
				<div>
					<ProductCardMetricLabel
						label="Agent Firewall"
						tooltip={agentFirewallTooltip}
					/>
					<div className="mt-0.5 text-sm font-medium text-content-primary">
						{isFirewallInUse ? "In use" : "Not in use"}
					</div>
				</div>
			</div>
			<div className="mt-4 text-sm">
				<Link asChild size="lg" showExternalIcon={false}>
					<RouterLink to="/ai/settings">AI settings</RouterLink>
				</Link>
			</div>
		</div>
	);
};

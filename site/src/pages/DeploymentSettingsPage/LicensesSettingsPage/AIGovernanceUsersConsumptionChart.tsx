import type { FC } from "react";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import {
	effectiveAiGovernanceLimitForUsageCard,
	hasAiGovernanceAddOnLicense,
} from "./AIGovernanceLicensing";
import { SeatUsageBarCard } from "./SeatUsageBarCard";

interface AIGovernanceUsersConsumptionProps {
	aiGovernanceUserFeature?: Feature;
	licenses?: GetLicensesResponse[];
}

export const AIGovernanceUsersConsumption: FC<
	AIGovernanceUsersConsumptionProps
> = ({ aiGovernanceUserFeature, licenses }) => {
	const hasAddOnLicense = hasAiGovernanceAddOnLicense(
		licenses,
		aiGovernanceUserFeature,
	);
	const effectiveLimit = effectiveAiGovernanceLimitForUsageCard(
		aiGovernanceUserFeature,
		licenses,
	);

	const showUsageBar =
		aiGovernanceUserFeature?.enabled === true ||
		(hasAddOnLicense && effectiveLimit !== undefined);

	if (!showUsageBar) {
		return (
			<div className="flex items-center justify-center rounded-lg border border-solid p-4">
				<div className="flex flex-col items-center justify-center">
					<div className="flex flex-col items-center justify-center">
						<span className="text-base">AI Governance add-on usage</span>
						<span className="text-content-secondary text-center max-w-[464px] mt-2">
							AI Governance is not included in your current license. Contact{" "}
							<Link href="mailto:sales@coder.com">sales</Link> to upgrade your
							license and unlock this addon.
						</span>
					</div>
				</div>
			</div>
		);
	}

	return (
		<SeatUsageBarCard
			title="AI Governance add-on usage"
			actual={aiGovernanceUserFeature?.actual}
			limit={effectiveLimit}
		/>
	);
};

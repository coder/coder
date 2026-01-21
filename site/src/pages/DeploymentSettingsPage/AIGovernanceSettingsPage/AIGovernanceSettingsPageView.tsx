import type { SerpentOption } from "api/typesGenerated";
import { Badges, PremiumBadge } from "components/Badges/Badges";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";

type AIGovernanceSettingsPageViewProps = {
	options: SerpentOption[];
	featureAIBridgeEnabled: boolean;
};

export const AIGovernanceSettingsPageView: FC<
	AIGovernanceSettingsPageViewProps
> = ({ options, featureAIBridgeEnabled }) => {
	return (
		<Stack direction="column" spacing={6}>
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>AI Governance</SettingsHeaderTitle>
				</SettingsHeader>

				<Badges>
					<Tooltip>
						<TooltipTrigger asChild>
							<span>
								<PremiumBadge />
							</span>
						</TooltipTrigger>

						<TooltipContent
							sideOffset={-28}
							collisionPadding={16}
							className="p-0"
						>
							<PopoverPaywall
								message="AI Governance"
								description="With a Premium license, you can monitor and manage AI requests across your deployment."
								documentationLink={docs("/ai-coder/ai-governance")}
							/>
						</TooltipContent>
					</Tooltip>
				</Badges>
			</div>

			{featureAIBridgeEnabled && (
				<div>
					<SettingsHeader
						actions={
							<SettingsHeaderDocsLink href={docs("/ai-coder/ai-bridge")} />
						}
					>
						<SettingsHeaderTitle hierarchy="secondary" level="h2">
							AI Bridge
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Monitor and manage AI requests across your deployment.
						</SettingsHeaderDescription>
					</SettingsHeader>

					<OptionsTable
						options={options
							.filter((o) => deploymentGroupHasParent(o.group, "AI Bridge"))
							.filter((o) => !o.annotations?.secret === true)}
					/>
				</div>
			)}
		</Stack>
	);
};

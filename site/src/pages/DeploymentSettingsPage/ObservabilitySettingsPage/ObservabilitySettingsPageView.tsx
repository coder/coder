import type { SerpentOption } from "api/typesGenerated";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "components/Badges/Badges";
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

type ObservabilitySettingsPageViewProps = {
	options: SerpentOption[];
	featureAuditLogEnabled: boolean;
	featureAIBridgeEnabled: boolean;
	isPremium: boolean;
};

export const ObservabilitySettingsPageView: FC<
	ObservabilitySettingsPageViewProps
> = ({
	options,
	featureAuditLogEnabled,
	isPremium,
	featureAIBridgeEnabled,
}) => {
	return (
		<Stack direction="column" spacing={6}>
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>Observability</SettingsHeaderTitle>
				</SettingsHeader>

				<SettingsHeader
					actions={
						<SettingsHeaderDocsLink href={docs("/admin/security/audit-logs")} />
					}
				>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						Audit Logging
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Allow auditors to monitor user operations in your deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Badges>
					<Tooltip>
						{featureAuditLogEnabled && !isPremium ? (
							<EnterpriseBadge />
						) : (
							<TooltipTrigger asChild>
								<span>
									<PremiumBadge />
								</span>
							</TooltipTrigger>
						)}

						<TooltipContent
							sideOffset={-28}
							collisionPadding={16}
							className="p-0"
						>
							<PopoverPaywall
								message="Observability"
								description="With a Premium license, you can monitor your application with logs and metrics."
								documentationLink="https://coder.com/docs/admin/appearance"
							/>
						</TooltipContent>
					</Tooltip>
				</Badges>
			</div>

			<div>
				<SettingsHeader>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						Monitoring
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Monitoring your Coder application with logs and metrics.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<OptionsTable
					options={options.filter((o) =>
						deploymentGroupHasParent(o.group, "Introspection"),
					)}
				/>
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
							.filter((o) => deploymentGroupHasParent(o.group, "AIBridge"))
							.filter((o) => !o.hidden)}
					/>
				</div>
			)}
		</Stack>
	);
};

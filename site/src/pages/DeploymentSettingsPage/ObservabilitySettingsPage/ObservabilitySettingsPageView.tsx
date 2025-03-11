import type { SerpentOption } from "api/typesGenerated";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "components/Badges/Badges";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import type { FC } from "react";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";

export type ObservabilitySettingsPageViewProps = {
	options: SerpentOption[];
	featureAuditLogEnabled: boolean;
	isPremium: boolean;
};

export const ObservabilitySettingsPageView: FC<
	ObservabilitySettingsPageViewProps
> = ({ options, featureAuditLogEnabled, isPremium }) => {
	return (
		<>
			<Stack direction="column" spacing={6}>
				<div>
					<SettingsHeader title="Observability" />
					<SettingsHeader
						title="Audit Logging"
						hierarchy="secondary"
						description="Allow auditors to monitor user operations in your deployment."
						docsHref={docs("/admin/security/audit-logs")}
					/>

					<Badges>
						<Popover mode="hover">
							{featureAuditLogEnabled && !isPremium ? (
								<EnterpriseBadge />
							) : (
								<PopoverTrigger>
									<span>
										<PremiumBadge />
									</span>
								</PopoverTrigger>
							)}

							<PopoverContent css={{ transform: "translateY(-28px)" }}>
								<PopoverPaywall
									message="Observability"
									description="With a Premium license, you can monitor your application with logs and metrics."
									documentationLink="https://coder.com/docs/admin/appearance"
								/>
							</PopoverContent>
						</Popover>
					</Badges>
				</div>

				<div>
					<SettingsHeader
						title="Monitoring"
						hierarchy="secondary"
						description="Monitoring your Coder application with logs and metrics."
					/>

					<OptionsTable
						options={options.filter((o) =>
							deploymentGroupHasParent(o.group, "Introspection"),
						)}
					/>
				</div>
			</Stack>
		</>
	);
};

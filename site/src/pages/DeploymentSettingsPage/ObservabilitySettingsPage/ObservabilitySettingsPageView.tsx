import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { deploymentGroupHasParent } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type ObservabilitySettingsPageViewProps = {
	options: SerpentOption[];
	featureAuditLogEnabled: boolean;
	canViewPremium: boolean;
};

export const ObservabilitySettingsPageView: FC<
	ObservabilitySettingsPageViewProps
> = ({ options, featureAuditLogEnabled, canViewPremium }) => {
	return (
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader
					actions={<SettingsHeaderDocsLink href={docs("/admin/monitoring")} />}
				>
					<SettingsHeaderTitle>Observability</SettingsHeaderTitle>
				</SettingsHeader>

				<SettingsHeader>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						Audit Logging
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Allow auditors to monitor user operations in your deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{featureAuditLogEnabled ? (
					<OptionsTable
						options={options.filter((o) => o.name === "Audit Logs Retention")}
					/>
				) : (
					<PaywallPremium
						message="Audit Logging"
						description="Audit logging lets auditors monitor user operations across your deployment. You need a Premium license to use this feature."
						canViewPremium={canViewPremium}
					/>
				)}
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
		</div>
	);
};

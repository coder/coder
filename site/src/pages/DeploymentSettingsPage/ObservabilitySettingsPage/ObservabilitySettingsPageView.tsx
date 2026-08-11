import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "#/components/Badges/Badges";
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
	isPremium: boolean;
};

export const ObservabilitySettingsPageView: FC<
	ObservabilitySettingsPageViewProps
> = ({ options, featureAuditLogEnabled, isPremium }) => {
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

				{featureAuditLogEnabled || isPremium ? (
					<Badges>{isPremium ? <PremiumBadge /> : <EnterpriseBadge />}</Badges>
				) : (
					<Alert severity="info" actions={<PremiumBadge />}>
						Audit logging lets auditors monitor user operations across your
						deployment. It requires a Premium license.{" "}
						<a
							href={docs("/admin/security/audit-logs")}
							target="_blank"
							rel="noreferrer"
							className="text-content-link font-medium"
						>
							Read the Audit Logs documentation
						</a>
						.
					</Alert>
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

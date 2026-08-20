import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import {
	Badges,
	DisabledBadge,
	EnabledBadge,
} from "#/components/Badges/Badges";
import { PaywallSmall } from "#/components/Paywall/PaywallSmall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	deploymentGroupHasParent,
	useDeploymentOptions,
} from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type SecuritySettingsPageViewProps = {
	options: SerpentOption[];
	featureBrowserOnlyEnabled: boolean;
};

export const SecuritySettingsPageView: FC<SecuritySettingsPageViewProps> = ({
	options,
	featureBrowserOnlyEnabled,
}) => {
	const tlsOptions = options.filter((o) =>
		deploymentGroupHasParent(o.group, "TLS"),
	);

	return (
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>Security</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Ensure your Coder deployment is secure.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<OptionsTable
					options={useDeploymentOptions(
						options,
						"SSH Keygen Algorithm",
						"Secure Auth Cookie",
						"Disable Owner Workspace Access",
					)}
				/>
			</div>

			<div>
				<SettingsHeader
					actions={
						<SettingsHeaderDocsLink
							href={docs("/admin/networking#browser-only-connections")}
						/>
					}
				>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Browser-Only Connections{" "}
						<Badges>
							{featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
						</Badges>
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Block all workspace access via SSH, port forward, and other
						non-browser connections.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Badges>
					{featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
				</Badges>
				{!featureBrowserOnlyEnabled ? (
					<PaywallSmall
						message="Browser-Only Connections"
						description="Block all workspace access via SSH, port forward, and other
						non-browser connections."
						canViewPremium
					/>
				) : null}
			</div>

			{tlsOptions.length > 0 && (
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							TLS
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Ensure TLS is properly configured for your Coder deployment.
						</SettingsHeaderDescription>
					</SettingsHeader>

					<OptionsTable options={tlsOptions} />
				</div>
			)}
		</div>
	);
};

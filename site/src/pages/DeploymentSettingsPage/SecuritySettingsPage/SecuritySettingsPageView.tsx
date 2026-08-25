import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import {
	Badges,
	DisabledBadge,
	EnabledBadge,
} from "#/components/Badges/Badges";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallSmall } from "#/modules/paywall/PremiumPaywallSmall";
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
						Ensure your Coder deployment is secure.{" "}
						<SettingsHeaderDocsLink href={docs("/admin/security")} />
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
				<SettingsHeader>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Browser-Only Connections{" "}
						<Badges>
							{featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
						</Badges>
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Block all workspace access via SSH, port forward, and other
						non-browser connections.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/networking#browser-only-connections")}
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Badges>
					{featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
				</Badges>
				{!featureBrowserOnlyEnabled ? (
					<PremiumPaywallSmall
						source="browser_only"
						message="Browser-Only Connections"
						description="Block all workspace access via SSH, port forward, and other non-browser connections."
						features={[
							"Restrict access to web-based connections",
							"Block SSH and port-forward entirely",
							"Enforce browser-only compliance policies",
						]}
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

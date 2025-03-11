import type { SerpentOption } from "api/typesGenerated";
import { Badges, DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import {
	deploymentGroupHasParent,
	useDeploymentOptions,
} from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";

export type UserAuthSettingsPageViewProps = {
	options: SerpentOption[];
};

export const UserAuthSettingsPageView = ({
	options,
}: UserAuthSettingsPageViewProps): JSX.Element => {
	const oidcEnabled = Boolean(
		useDeploymentOptions(options, "OIDC Client ID")[0].value,
	);
	const githubEnabled = Boolean(
		useDeploymentOptions(options, "OAuth2 GitHub Client ID")[0].value,
	);

	return (
		<>
			<Stack direction="column" spacing={6}>
				<div>
					<SettingsHeader title="User Authentication" />

					<SettingsHeader
						title="Login with OpenID Connect"
						hierarchy="secondary"
						description="Set up authentication to login with OpenID Connect."
						docsHref={docs("/admin/users/oidc-auth#openid-connect")}
					/>

					<Badges>{oidcEnabled ? <EnabledBadge /> : <DisabledBadge />}</Badges>

					{oidcEnabled && (
						<OptionsTable
							options={options.filter((o) =>
								deploymentGroupHasParent(o.group, "OIDC"),
							)}
						/>
					)}
				</div>

				<div>
					<SettingsHeader
						title="Login with GitHub"
						hierarchy="secondary"
						description="Set up authentication to login with GitHub."
						docsHref={docs("/admin/users/github-auth")}
					/>

					<Badges>
						{githubEnabled ? <EnabledBadge /> : <DisabledBadge />}
					</Badges>

					{githubEnabled && (
						<OptionsTable
							options={options.filter((o) =>
								deploymentGroupHasParent(o.group, "GitHub"),
							)}
						/>
					)}
				</div>
			</Stack>
		</>
	);
};

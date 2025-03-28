import type { SerpentOption } from "api/typesGenerated";
import { Badges, DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
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
					<SettingsHeader>
						<SettingsHeaderTitle>User Authentication</SettingsHeaderTitle>
					</SettingsHeader>

					<SettingsHeader
						actions={
							<SettingsHeaderDocsLink
								href={docs("/admin/users/oidc-auth#openid-connect")}
							/>
						}
					>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							Login with OpenID Connect
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Set up authentication to login with OpenID Connect.
						</SettingsHeaderDescription>
					</SettingsHeader>

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
						actions={
							<SettingsHeaderDocsLink href={docs("/admin/users/github-auth")} />
						}
					>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							Login with GitHub
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Set up authentication to login with GitHub.
						</SettingsHeaderDescription>
					</SettingsHeader>

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

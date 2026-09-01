import type { JSX } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import {
	BadgeGroup,
	DisabledBadge,
	EnabledBadge,
} from "#/components/Badge/PresetBadges";
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

type UserAuthSettingsPageViewProps = {
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
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>User Authentication</SettingsHeaderTitle>
				</SettingsHeader>

				<SettingsHeader>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Login with OpenID Connect
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Set up authentication to login with OpenID Connect.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/users/oidc-auth")}
							context="about OpenID Connect login"
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<BadgeGroup>
					{oidcEnabled ? <EnabledBadge /> : <DisabledBadge />}
				</BadgeGroup>

				{oidcEnabled && (
					<OptionsTable
						options={options.filter((o) =>
							deploymentGroupHasParent(o.group, "OIDC"),
						)}
					/>
				)}
			</div>

			<div>
				<SettingsHeader>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Login with GitHub
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Set up authentication to login with GitHub.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/users/github-auth")}
							context="about GitHub login"
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<BadgeGroup>
					{githubEnabled ? <EnabledBadge /> : <DisabledBadge />}
				</BadgeGroup>

				{githubEnabled && (
					<OptionsTable
						options={options.filter((o) =>
							deploymentGroupHasParent(o.group, "GitHub"),
						)}
					/>
				)}
			</div>
		</div>
	);
};

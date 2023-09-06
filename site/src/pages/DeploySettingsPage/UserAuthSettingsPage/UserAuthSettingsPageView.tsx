import { DeploymentOption } from "api/types";
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
} from "components/DeploySettingsLayout/Badges";
import { Header } from "components/DeploySettingsLayout/Header";
import OptionsTable from "components/DeploySettingsLayout/OptionsTable";
import { Stack } from "components/Stack/Stack";
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "utils/deployOptions";
import { docs } from "utils/docs";

export type UserAuthSettingsPageViewProps = {
  options: DeploymentOption[];
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
          <Header title="User Authentication" />

          <Header
            title="Login with OpenID Connect"
            secondary
            description="Set up authentication to login with OpenID Connect."
            docsHref={docs("/admin/auth#openid-connect-with-google")}
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
          <Header
            title="Login with GitHub"
            secondary
            description="Set up authentication to login with GitHub."
            docsHref={docs("/admin/auth#github")}
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

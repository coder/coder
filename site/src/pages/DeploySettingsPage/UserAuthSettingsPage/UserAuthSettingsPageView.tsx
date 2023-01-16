import { DeploymentConfig } from "api/typesGenerated"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"

export type UserAuthSettingsPageViewProps = {
  deploymentConfig: Pick<DeploymentConfig, "oidc" | "oauth2">
}

export const UserAuthSettingsPageView = ({
  deploymentConfig,
}: UserAuthSettingsPageViewProps): JSX.Element => (
  <>
    <Stack direction="column" spacing={6}>
      <div>
        <Header title="User Authentication" />

        <Header
          title="Login with OpenID Connect"
          secondary
          description="Set up authentication to login with OpenID Connect."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
        />

        <Badges>
          {deploymentConfig.oidc.client_id.value ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
        </Badges>

        <OptionsTable
          options={{
            client_id: deploymentConfig.oidc.client_id,
            allow_signups: deploymentConfig.oidc.allow_signups,
            email_domain: deploymentConfig.oidc.email_domain,
            issuer_url: deploymentConfig.oidc.issuer_url,
            scopes: deploymentConfig.oidc.scopes,
          }}
        />
      </div>

      <div>
        <Header
          title="Login with GitHub"
          secondary
          description="Set up authentication to login with GitHub."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#github"
        />

        <Badges>
          {deploymentConfig.oauth2.github.client_id.value ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
        </Badges>

        <OptionsTable
          options={{
            client_id: deploymentConfig.oauth2.github.client_id,
            allow_signups: deploymentConfig.oauth2.github.allow_signups,
            allowed_orgs: deploymentConfig.oauth2.github.allowed_orgs,
            allowed_teams: deploymentConfig.oauth2.github.allowed_teams,
            enterprise_base_url:
              deploymentConfig.oauth2.github.enterprise_base_url,
          }}
        />
      </div>
    </Stack>
  </>
)

import {
  Badges,
  DisabledBadge,
  EnabledBadge,
} from "components/DeploySettingsLayout/Badges"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"

const AuthSettingsPage: React.FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Authentication Settings")}</title>
      </Helmet>

      <Stack direction="column" spacing={6}>
        <div>
          <Header title="Authentication" />

          <Header
            title="Login with OpenID Connect"
            secondary
            description="Set up authentication to login with OpenID Connect."
            docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
          />

          <Badges>
            {deploymentConfig.oidc_client_id.value ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
          </Badges>

          <OptionsTable
            options={{
              oidc_client_id: deploymentConfig.oidc_client_id,
              oidc_client_secret: deploymentConfig.oidc_client_secret,
              oidc_allow_signups: deploymentConfig.oidc_allow_signups,
              oidc_email_domain: deploymentConfig.oidc_email_domain,
              oidc_issuer_url: deploymentConfig.oidc_issuer_url,
              oidc_scopes: deploymentConfig.oidc_scopes,
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
            {deploymentConfig.oauth2_github_client_id.value ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
          </Badges>

          <OptionsTable
            options={{
              oauth2_github_client_id: deploymentConfig.oauth2_github_client_id,
              oauth2_github_client_secret:
                deploymentConfig.oauth2_github_client_secret,
              oauth2_github_allow_signups:
                deploymentConfig.oauth2_github_allow_signups,
              oauth2_github_allowed_orgs:
                deploymentConfig.oauth2_github_allowed_orgs,
              oauth2_github_allowed_teams:
                deploymentConfig.oauth2_github_allowed_teams,
              oauth2_github_enterprise_base_url:
                deploymentConfig.oauth2_github_enterprise_base_url,
            }}
          />
        </div>
      </Stack>
    </>
  )
}

export default AuthSettingsPage

import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import { Stack } from "components/Stack/Stack"
import React from "react"

export const AuthSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()

  return (
    <>
      <Stack direction="column" spacing={6}>
        <div>
          <Header
            title="GitHub"
            description="Authentication settings for GitHub"
            docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
          />

          <Badges>
            {deploymentFlags.oauth2_github_allow_signups.value ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
          </Badges>

          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="50%">Option</TableCell>
                  <TableCell width="50%">Value</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <TableRow>
                  <TableCell>
                    <OptionName>
                      {deploymentFlags.oauth2_github_client_id.name}
                    </OptionName>
                    <OptionDescription>
                      {deploymentFlags.oauth2_github_client_id.description}
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>
                      {deploymentFlags.oauth2_github_client_id.value}
                    </OptionValue>
                  </TableCell>
                </TableRow>

                <TableRow>
                  <TableCell>
                    <OptionName>
                      {deploymentFlags.oauth2_github_client_secret.name}
                    </OptionName>
                    <OptionDescription>
                      {deploymentFlags.oauth2_github_client_secret.description}
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>
                      {deploymentFlags.oauth2_github_client_secret.value}
                    </OptionValue>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        </div>

        <div>
          <Header
            title="OIDC"
            description="Authentication settings for GitHub"
            docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
          />

          <Badges>
            {deploymentFlags.oidc_allow_signups.value ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
            <EnterpriseBadge />
          </Badges>

          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="50%">Option</TableCell>
                  <TableCell width="50%">Value</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <TableRow>
                  <TableCell>
                    <OptionName>
                      {deploymentFlags.oidc_client_id.name}
                    </OptionName>
                    <OptionDescription>
                      {deploymentFlags.oidc_client_id.description}
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>
                      {deploymentFlags.oidc_client_id.value}
                    </OptionValue>
                  </TableCell>
                </TableRow>

                <TableRow>
                  <TableCell>
                    <OptionName>
                      {deploymentFlags.oidc_cliet_secret.name}
                    </OptionName>
                    <OptionDescription>
                      {deploymentFlags.oidc_cliet_secret.description}
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>
                      {deploymentFlags.oidc_cliet_secret.value}
                    </OptionValue>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        </div>
      </Stack>
    </>
  )
}

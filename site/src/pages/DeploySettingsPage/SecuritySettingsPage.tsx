import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
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
import React, { useContext } from "react"
import { XServiceContext } from "xServices/StateContext"

const SecuritySettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()
  const xServices = useContext(XServiceContext)
  const [entitlementsState] = useActor(xServices.entitlementsXService)

  return (
    <Stack direction="column" spacing={6}>
      <div>
        <Header
          title="Security"
          description="Ensure your Coder deployment is secure."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
        />

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
                    {deploymentFlags.ssh_keygen_algorithm.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.ssh_keygen_algorithm.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.ssh_keygen_algorithm.value}
                  </OptionValue>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <OptionName>
                    {deploymentFlags.secure_auth_cookie.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.secure_auth_cookie.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.secure_auth_cookie.value ? (
                      <EnabledBadge />
                    ) : (
                      <DisabledBadge />
                    )}
                  </OptionValue>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </div>

      <div>
        <Header
          title="Audit Logging"
          secondary
          description="Allow auditors to monitor user operations in your deployment."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/audit-logs"
        />

        <Badges>
          {entitlementsState.context.entitlements.features[
            FeatureNames.AuditLog
          ].enabled ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
          <EnterpriseBadge />
        </Badges>
      </div>

      <div>
        <Header
          title="Browser Only Connections"
          secondary
          description="Block all workspace access via SSH, port forward, and other non-browser connections."
          docsHref="https://coder.com/docs/coder-oss/latest/networking#browser-only-connections-enterprise"
        />

        <Badges>
          {entitlementsState.context.entitlements.features[
            FeatureNames.BrowserOnly
          ].enabled ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
          <EnterpriseBadge />
        </Badges>
      </div>

      <div>
        <Header
          title="TLS"
          secondary
          description="Ensure TLS is properly configured for your Coder deployment."
        />

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
                  <OptionName>{deploymentFlags.tls_enable.name}</OptionName>
                  <OptionDescription>
                    {deploymentFlags.tls_enable.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.tls_enable.value ? (
                      <EnabledBadge />
                    ) : (
                      <DisabledBadge />
                    )}
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>{deploymentFlags.tls_cert_files.name}</OptionName>
                  <OptionDescription>
                    {deploymentFlags.tls_cert_files.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    <ul>
                      {deploymentFlags.tls_cert_files.value.map(
                        (file, index) => (
                          <li key={index}>{file}</li>
                        ),
                      )}
                    </ul>
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>{deploymentFlags.tls_key_files.name}</OptionName>
                  <OptionDescription>
                    {deploymentFlags.tls_key_files.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    <ul>
                      {deploymentFlags.tls_key_files.value.map(
                        (file, index) => (
                          <li key={index}>{file}</li>
                        ),
                      )}
                    </ul>
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>
                    {deploymentFlags.tls_min_version.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.tls_min_version.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.tls_min_version.value}
                  </OptionValue>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </div>
    </Stack>
  )
}

export default SecuritySettingsPage

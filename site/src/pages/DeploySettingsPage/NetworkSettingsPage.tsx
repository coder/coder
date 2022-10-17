import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import {
  DisabledBadge,
  EnabledBadge,
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

const NetworkSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()

  return (
    <Stack direction="column" spacing={6}>
      <div>
        <Header
          title="Network"
          description="Configure your deployment connectivity."
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
                    {deploymentFlags.derp_server_enabled.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.derp_server_enabled.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.derp_server_enabled.value ? (
                      <EnabledBadge />
                    ) : (
                      <DisabledBadge />
                    )}
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>
                    {deploymentFlags.derp_server_region_name.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.derp_server_region_name.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.derp_server_region_name.value}
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>
                    {deploymentFlags.derp_server_stun_address.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.derp_server_stun_address.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.derp_server_stun_address.value}
                  </OptionValue>
                </TableCell>
              </TableRow>

              <TableRow>
                <TableCell>
                  <OptionName>
                    {deploymentFlags.derp_config_url.name}
                  </OptionName>
                  <OptionDescription>
                    {deploymentFlags.derp_config_url.description}
                  </OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {deploymentFlags.derp_config_url.value}
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

export default NetworkSettingsPage

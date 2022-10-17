import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { useMachine } from "@xstate/react"
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
import { ReplicasTable } from "components/ReplicasTable/ReplicasTable"
import { Stack } from "components/Stack/Stack"
import React from "react"
import { highAvailabilityMachine } from "xServices/deploymentFlags/highAvailabilityMachine"

const NetworkSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()
  const [state] = useMachine(highAvailabilityMachine)

  return (
    <Stack direction="column" spacing={6}>
      <div>
        <Header
          title="Network"
          description="Deployment and networking settings"
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

      <div>
        <Header
          title="High Availability"
          secondary
          description="Deploy Coder multi regionally for high availability. Only active if more than one replica exists."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
        />

        <Badges>
          {deploymentFlags.derp_server_relay_address.value ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
          <EnterpriseBadge />
        </Badges>

        <ReplicasTable replicas={state.context.replicas || []} />
      </div>
    </Stack>
  )
}

export default NetworkSettingsPage

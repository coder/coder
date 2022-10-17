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
import React from "react"

export const MetricsSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()

  return (
    <>
      <Header
        title="Metrics"
        description="Metrics settings"
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
                <OptionName>{deploymentFlags.telemetry_enable.name}</OptionName>
                <OptionDescription>
                  {deploymentFlags.telemetry_enable.description}
                </OptionDescription>
              </TableCell>

              <TableCell>
                <OptionValue>
                  {deploymentFlags.telemetry_enable.value ? (
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
    </>
  )
}

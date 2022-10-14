import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import {
  DeploySettingsLayout,
  SettingsHeader,
} from "components/DeploySettingsLayout/DeploySettingsLayout"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import React from "react"

export const MetricsSettingsPage: React.FC = () => {
  return (
    <DeploySettingsLayout>
      <SettingsHeader
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
                <OptionName>Telemetry Enabled</OptionName>
                <OptionDescription>Some description</OptionDescription>
              </TableCell>

              <TableCell>
                <OptionValue>Yes</OptionValue>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
    </DeploySettingsLayout>
  )
}

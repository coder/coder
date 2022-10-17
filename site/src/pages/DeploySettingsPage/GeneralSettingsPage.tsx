import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import React from "react"

const GeneralSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()

  return (
    <>
      <Header
        title="General"
        description="Settings for accessing your Coder deployment."
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
                <OptionName>{deploymentFlags.access_url.name}</OptionName>
                <OptionDescription>
                  {deploymentFlags.access_url.description}
                </OptionDescription>
              </TableCell>

              <TableCell>
                <OptionValue>{deploymentFlags.access_url.value}</OptionValue>
              </TableCell>
            </TableRow>

            <TableRow>
              <TableCell>
                <OptionName>{deploymentFlags.address.name}</OptionName>
                <OptionDescription>
                  {deploymentFlags.address.description}
                </OptionDescription>
              </TableCell>

              <TableCell>
                <OptionValue>{deploymentFlags.address.value}</OptionValue>
              </TableCell>
            </TableRow>

            <TableRow>
              <TableCell>
                <OptionName>
                  {deploymentFlags.wildcard_access_url.name}
                </OptionName>
                <OptionDescription>
                  {deploymentFlags.wildcard_access_url.description}
                </OptionDescription>
              </TableCell>

              <TableCell>
                <OptionValue>
                  {deploymentFlags.wildcard_access_url.value}
                </OptionValue>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
    </>
  )
}

export default GeneralSettingsPage

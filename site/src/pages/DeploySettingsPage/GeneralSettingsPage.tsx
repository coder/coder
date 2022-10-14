import { makeStyles } from "@material-ui/core/styles"
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
import React from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

export const GeneralSettingsPage: React.FC = () => {
  const styles = useStyles()

  return (
    <DeploySettingsLayout>
      <div>
        <SettingsHeader
          title="General"
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
                  <span className={styles.optionName}>Access URL</span>
                  <span className={styles.optionDescription}>
                    The address to serve the API and dashboard.
                  </span>
                </TableCell>

                <TableCell>
                  <span className={styles.optionValue}>127.0.0.1:3000</span>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <span className={styles.optionName}>Wildcard Access URL</span>
                  <span className={styles.optionDescription}>
                    Specifies the external URL to access Coder.
                  </span>
                </TableCell>

                <TableCell>
                  <span className={styles.optionValue}>
                    https://www.dev.coder.com
                  </span>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </div>
    </DeploySettingsLayout>
  )
}

const useStyles = makeStyles((theme) => ({
  optionName: {
    display: "block",
  },
  optionDescription: {
    display: "block",
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(0.5),
  },
  optionValue: {
    fontSize: 14,
    fontFamily: MONOSPACE_FONT_FAMILY,
  },
}))

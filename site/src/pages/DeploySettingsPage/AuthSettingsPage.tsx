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
import {
  DeploySettingsLayout,
  SettingsHeader,
} from "components/DeploySettingsLayout/DeploySettingsLayout"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import { Stack } from "components/Stack/Stack"
import React from "react"

export const AuthSettingsPage: React.FC = () => {
  return (
    <DeploySettingsLayout>
      <Stack direction="column" spacing={6}>
        <div>
          <SettingsHeader
            title="GitHub"
            description="Authentication settings for GitHub"
            docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
          />

          <Badges>
            <EnabledBadge />
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
                    <OptionName>Client ID</OptionName>
                    <OptionDescription>
                      GitHub client ID for OAuth
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>asjdalsj-9u129jalksjlakjsd</OptionValue>
                  </TableCell>
                </TableRow>

                <TableRow>
                  <TableCell>
                    <OptionName>Client Secret</OptionName>
                    <OptionDescription>
                      GitHub client secret for OAuth
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>Not available</OptionValue>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        </div>

        <div>
          <SettingsHeader
            title="OIDC"
            description="Authentication settings for GitHub"
            docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
          />

          <Badges>
            <DisabledBadge />
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
                    <OptionName>Client ID</OptionName>
                    <OptionDescription>
                      GitHub client ID for OAuth
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>asjdalsj-9u129jalksjlakjsd</OptionValue>
                  </TableCell>
                </TableRow>

                <TableRow>
                  <TableCell>
                    <OptionName>Client Secret</OptionName>
                    <OptionDescription>
                      GitHub client secret for OAuth
                    </OptionDescription>
                  </TableCell>

                  <TableCell>
                    <OptionValue>Not available</OptionValue>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        </div>
      </Stack>
    </DeploySettingsLayout>
  )
}

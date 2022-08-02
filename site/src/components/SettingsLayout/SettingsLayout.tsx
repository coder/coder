import Box from "@material-ui/core/Box"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { Outlet } from "react-router-dom"
import { pageTitle } from "../../util/page"
import { AuthAndFrame } from "../AuthAndFrame/AuthAndFrame"
import { Margins } from "../Margins/Margins"
import { TabPanel } from "../TabPanel/TabPanel"

export const Language = {
  accountLabel: "Account",
  securityLabel: "Security",
  sshKeysLabel: "SSH keys",
  settingsLabel: "Settings",
}

const menuItems = [
  { label: Language.accountLabel, path: "/settings/account" },
  { label: Language.securityLabel, path: "/settings/security" },
  { label: Language.sshKeysLabel, path: "/settings/ssh-keys" },
]

export const SettingsLayout: FC<React.PropsWithChildren<unknown>> = () => {
  return (
    <AuthAndFrame>
      <Box display="flex" flexDirection="column">
        <Helmet>
          <title>{pageTitle("Settings")}</title>
        </Helmet>
        <Margins>
          <TabPanel title={Language.settingsLabel} menuItems={menuItems}>
            <Outlet />
          </TabPanel>
        </Margins>
      </Box>
    </AuthAndFrame>
  )
}

import Box from "@material-ui/core/Box"
import React from "react"
import { Outlet } from "react-router-dom"
import { AuthAndFrame } from "../AuthAndFrame/AuthAndFrame"
import { Margins } from "../Margins/Margins"
import { TabPanel } from "../TabPanel/TabPanel"

export const Language = {
  accountLabel: "Account",
  sshKeysLabel: "SSH keys",
  settingsLabel: "Settings",
}

const menuItems = [
  { label: Language.accountLabel, path: "/settings/account" },
  { label: Language.sshKeysLabel, path: "/settings/ssh-keys" },
]

export const SettingsLayout: React.FC = () => {
  return (
    <AuthAndFrame>
      <Box display="flex" flexDirection="column">
        <Margins>
          <TabPanel title={Language.settingsLabel} menuItems={menuItems}>
            <Outlet />
          </TabPanel>
        </Margins>
      </Box>
    </AuthAndFrame>
  )
}

import Box from "@material-ui/core/Box"
import React from "react"
import { Outlet } from "react-router-dom"
import { AuthAndFrame } from "../AuthAndFrame/AuthAndFrame"
import { Margins } from "../Margins/Margins"
import { TabPanel } from "../TabPanel/TabPanel"

export const Language = {
  accountLabel: "Account",
  sshKeysLabel: "SSH Keys",
  preferencesLabel: "Preferences",
}

const menuItems = [
  { label: Language.accountLabel, path: "/preferences/account" },
  { label: Language.sshKeysLabel, path: "/preferences/ssh-keys" },
]

export const PreferencesLayout: React.FC = () => {
  return (
    <AuthAndFrame>
      <Box display="flex" flexDirection="column">
        <Margins>
          <TabPanel title={Language.preferencesLabel} menuItems={menuItems}>
            <Outlet />
          </TabPanel>
        </Margins>
      </Box>
    </AuthAndFrame>
  )
}

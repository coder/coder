import Box from "@material-ui/core/Box"
import React from "react"
import { Outlet } from "react-router-dom"
import { AuthAndFrame } from "../AuthAndFrame/AuthAndFrame"
import { TabPanel } from "../TabPanel"

export const Language = {
  accountLabel: "Account",
  securityLabel: "Security",
  sshKeysLabel: "SSH Keys",
  linkedAccountsLabel: "Linked Accounts",
  preferencesLabel: "Preferences",
}

const menuItems = [
  { label: Language.accountLabel, path: "/preferences/account" },
  { label: Language.securityLabel, path: "/preferences/security" },
  { label: Language.sshKeysLabel, path: "/preferences/ssh-keys" },
  { label: Language.linkedAccountsLabel, path: "/preferences/linked-accounts" },
]

export const PreferencesLayout: React.FC = () => {
  return (
    <AuthAndFrame>
      <Box display="flex" flexDirection="column">
        <Box style={{ maxWidth: "1380px", margin: "1em auto" }}>
          <TabPanel title={Language.preferencesLabel} menuItems={menuItems}>
            <Outlet />
          </TabPanel>
        </Box>
      </Box>
    </AuthAndFrame>
  )
}

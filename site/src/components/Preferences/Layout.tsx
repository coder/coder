import Box from "@material-ui/core/Box"
import React from "react"
import { Outlet } from "react-router-dom"
import { AuthAndNav } from "../Page"
import { TabPanel } from "../TabPanel"

const menuItems = [
  { label: "Account", path: "/preferences/account" },
  { label: "Security", path: "/preferences/security" },
  { label: "SSH Keys", path: "/preferences/ssh-keys" },
  { label: "Linked Accounts", path: "/preferences/linked-accounts" },
]

export const PreferencesLayout: React.FC = () => {
  return (
    <AuthAndNav>
      <Box display="flex" flexDirection="column">
        <Box style={{ maxWidth: "1380px", margin: "1em auto" }}>
          <TabPanel title="Preferences" menuItems={menuItems}>
            <Outlet />
          </TabPanel>
        </Box>
      </Box>
    </AuthAndNav>
  )
}

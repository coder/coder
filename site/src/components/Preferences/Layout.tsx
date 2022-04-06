import Box from "@material-ui/core/Box"
import React from "react"
import { TabPanel } from "../TabPanel"

const menuItems = [
  { label: "Account", path: "/preferences/account" },
  { label: "Security", path: "/preferences/security" },
  { label: "SSH Keys", path: "/preferences/ssh-keys" },
  { label: "Linked Accounts", path: "/preferences/linked-accounts" },
]

export const Layout: React.FC = ({ children }) => {
  return (
    <Box style={{ maxWidth: "1380px", margin: "1em auto" }}>
      <TabPanel title="Preferences" menuItems={menuItems}>
        {children}
      </TabPanel>
    </Box>
  )
}

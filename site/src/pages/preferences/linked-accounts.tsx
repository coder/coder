import Box from "@material-ui/core/Box"
import React from "react"
import { Layout } from "../../components/Preferences/Layout"
import { Section } from "../../components/Section"

export const PreferencesLinkedAccountsPage: React.FC = () => {
  return (
    <Box display="flex" flexDirection="column">
      <Layout>
        <Section
          title="Linked Accounts"
          description="Linking your Coder account will add your workspace SSH key, allowing you to perform Git actions on all your workspaces."
        />
      </Layout>
    </Box>
  )
}

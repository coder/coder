import Box from "@material-ui/core/Box"
import React from "react"
import { Layout } from "../../components/Preferences/Layout"
import { Section } from "../../components/Section"

export const PreferencesSecurityPage: React.FC = () => {
  return (
    <Box display="flex" flexDirection="column">
      <Layout>
        <Section title="Security" description="Changing your password will sign you out of your current session." />
      </Layout>
    </Box>
  )
}

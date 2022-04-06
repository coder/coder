import Box from "@material-ui/core/Box"
import React from "react"
import { Layout } from "../../components/Preferences/Layout"
import { Section } from "../../components/Section"

export const PreferencesAccountPage: React.FC = () => {
  return (
    <Box display="flex" flexDirection="column">
      <Layout>
        <Section
          title="Account"
          description="Update your display name, email, profile picture, and dotfiles preferences."
        />
      </Layout>
    </Box>
  )
}

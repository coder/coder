import Box from "@material-ui/core/Box"
import React from "react"
import { Layout } from "../../components/Preferences/Layout"
import { Section } from "../../components/Section"

export const PreferencesSSHKeysPage: React.FC = () => {
  return (
    <Box display="flex" flexDirection="column">
      <Layout>
        <Section
          title="SSH Keys"
          description="Coder automatically inserts a private key into every workspace; you can add the corresponding public key to any services (such as Git) that you need access to from your workspace."
        />
      </Layout>
    </Box>
  )
}

import React from "react"
import { Section } from "../../components/Section"

export const PreferencesSSHKeysPage: React.FC = () => {
  return (
    <Section
      title="SSH Keys"
      description="Coder automatically inserts a private key into every workspace; you can add the corresponding public key to any services (such as Git) that you need access to from your workspace."
    />
  )
}

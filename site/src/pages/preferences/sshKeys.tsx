import React from "react"
import { Section } from "../../components/Section/Section"

const Language = {
  title: "SSH Keys",
  description:
    "Coder automatically inserts a private key into every workspace; you can add the corresponding public key to any services (such as Git) that you need access to from your workspace.",
}

export const PreferencesSSHKeysPage: React.FC = () => {
  return <Section title={Language.title} description={Language.description} />
}

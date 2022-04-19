import React from "react"
import { Section } from "../../components/Section"

const Language = {
  title: "Linked Accounts",
  description:
    "Linking your Coder account will add your workspace SSH key, allowing you to perform Git actions on all your workspaces.",
}

export const PreferencesLinkedAccountsPage: React.FC = () => {
  return <Section title={Language.title} description={Language.description} />
}

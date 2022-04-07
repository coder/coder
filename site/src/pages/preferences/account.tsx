import React from "react"
import { Section } from "../../components/Section"

const Language = {
  title: "Account",
  description: "Update your display name, email, profile picture, and dotfiles preferences.",
}

export const PreferencesAccountPage: React.FC = () => {
  return <Section title={Language.title} description={Language.description} />
}

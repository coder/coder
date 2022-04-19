import React from "react"
import { Section } from "../../../components/Section/Section"

const Language = {
  title: "Security",
  description: "Changing your password will sign you out of your current session.",
}

export const SecurityPage: React.FC = () => {
  return <Section title={Language.title} description={Language.description} />
}

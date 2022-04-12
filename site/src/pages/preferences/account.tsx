import React from "react"
import { AccountForm } from "../../components/Preferences/AccountForm"
import { Section } from "../../components/Section"

const Language = {
  title: "Account",
  description: "Update your display name, email and username.",
}

export const PreferencesAccountPage: React.FC = () => {
  return (
    <>
      <Section title={Language.title} description={Language.description}>
        <AccountForm
          isLoading={false}
          initialValues={{ name: "Bruno", username: "bruno", email: "bruno@coder.com" }}
          onSubmit={async (values) => {
            console.info(values)
          }}
        />
      </Section>
    </>
  )
}

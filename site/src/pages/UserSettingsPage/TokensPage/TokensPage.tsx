import React from "react"
import { Section } from "../../../components/Section/Section"
import { TokensPageView } from "./TokensPageView"

export const Language = {
  title: "Authentication Tokens",
  description: (
    <p>
      The following public key is used to authenticate Git in workspaces. You
      may add it to Git services (such as GitHub) that you need to access from
      your workspace. <br />
      <br />
      Coder configures authentication via <code>$GIT_SSH_COMMAND</code>.
    </p>
  ),
}

export const TokensPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  return (
    <>
      <Section title={Language.title} description={Language.description}>
        <TokensPageView
          tokens={[]}
        />
      </Section>
    </>
  )
}

export default TokensPage

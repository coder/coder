import Button from "@material-ui/core/Button"
import Add from "@material-ui/icons/Add"
import React from "react"
import { Section } from "../../../components/Section/Section"
import { TokensPageView } from "./TokensPageView"

export const Language = {
  title: "Authentication Tokens",
  description: (
    <p>
      Authentication tokens are used to authenticate with the Coder API.
    </p>
  ),
}

export const TokensPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  return (
    <>
      <Section
        title={Language.title}
        description={Language.description}
        layout="fluid"
        toolbar={
          <Button
            startIcon={<Add />}
          >
            New Token
          </Button>
        }
      >
      <TokensPageView
          tokens={[
            {
              "id":"tBoVE3dqLl",
              "user_id":"f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
              "last_used":"0001-01-01T00:00:00Z",
              "expires_at":"2023-01-15T20:10:45.637438Z",
              "created_at":"2022-12-16T20:10:45.637452Z",
              "updated_at":"2022-12-16T20:10:45.637452Z",
              "login_type":"token",
              "scope":"all",
              "lifetime_seconds":2592000,
            },
            {
              "id":"tBoVE3dqLl",
              "user_id":"f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
              "last_used":"0001-01-01T00:00:00Z",
              "expires_at":"2023-01-15T20:10:45.637438Z",
              "created_at":"2022-12-16T20:10:45.637452Z",
              "updated_at":"2022-12-16T20:10:45.637452Z",
              "login_type":"token",
              "scope":"all",
              "lifetime_seconds":2592000,
            }
          ]}
        />
      </Section>
    </>
  )
}

export default TokensPage

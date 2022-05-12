import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers/renderHelpers"
import { CliAuthToken } from "./CliAuthToken"

describe("CliAuthToken", () => {
  it("renders content", async () => {
    // When
    render(<CliAuthToken sessionToken="test-token" />)

    // Then
    await screen.findByText("Session Token")
    await screen.findByText("test-token")
  })
})

import React from "react"
import { screen } from "@testing-library/react"
import { render } from "../../test_helpers"

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

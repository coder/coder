import React from "react"
import { screen } from "@testing-library/react"

import { render } from "../../test_helpers"
import { Page } from "./index"

describe("Page", () => {
  it("renders content", async () => {
    // When
    render(
      <Page>
        <div>Testing123</div>
      </Page>,
    )

    // Then
    await screen.findByText("Testing123")
  })
})

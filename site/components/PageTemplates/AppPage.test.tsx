import React from "react"
import { screen } from "@testing-library/react"

import { render } from "../../test_helpers"
import { AppPage } from "./AppPage"

describe("AppPage", () => {
  it("renders content", async () => {
    // When
    render(
      <AppPage>
        <div>Hello, World</div>H
      </AppPage>,
    )

    // Then
    // Content should render
    await screen.findByText("Hello, World", { exact: false })
    // Footer should render
    await screen.findByText("Copyright", { exact: false })
  })
})

import React from "react"
import { screen } from "@testing-library/react"

import { render } from "../../test_helpers"
import { Navbar } from "./index"

describe("Navbar", () => {
  it("renders content", async () => {
    // When
    render(<Navbar />)

    // Then
    await screen.findAllByText("Coder", { exact: false })
  })
})

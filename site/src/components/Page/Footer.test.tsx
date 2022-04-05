import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../test_helpers"
import { Footer } from "./Footer"

describe("Footer", () => {
  it("renders content", async () => {
    // When
    render(<Footer />)

    // Then
    await screen.findByText("Copyright", { exact: false })
  })
})

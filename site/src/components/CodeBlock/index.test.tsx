import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers"
import { CodeBlock } from "./index"

describe("CodeBlock", () => {
  it("renders lines)", async () => {
    // When
    render(<CodeBlock lines={["line1", "line2"]} />)

    // Then
    // Both lines should be rendered
    await screen.findByText("line1")
    await screen.findByText("line2")
  })
})

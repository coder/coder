import { render, screen } from "@testing-library/react"
import React from "react"
import { FullScreenLoader } from "./FullScreenLoader"

describe("FullScreenLoader", () => {
  it("renders", async () => {
    // When
    render(<FullScreenLoader />)

    // Then
    const element = await screen.findByRole("progressbar")
    expect(element).toBeDefined()
  })
})

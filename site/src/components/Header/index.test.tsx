import { screen } from "@testing-library/react"
import { render } from "./../../test_helpers"
import React from "react"
import { Header } from "./index"

describe("Header", () => {
  it("renders title and subtitle", async () => {
    // When
    render(<Header title="Title Test" subTitle="Subtitle Test" />)

    // Then
    const titleElement = await screen.findByText("Title Test")
    expect(titleElement).toBeDefined()

    const subTitleElement = await screen.findByText("Subtitle Test")
    expect(subTitleElement).toBeDefined()
  })

  it("renders button if specified", async () => {
    // When
    render(<Header title="Title" action={{ text: "Button Test" }} />)

    // Then
    const buttonElement = await screen.findByRole("button")
    expect(buttonElement).toBeDefined()
    expect(buttonElement.textContent).toEqual("Button Test")
  })
})

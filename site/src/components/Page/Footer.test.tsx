import { screen } from "@testing-library/react"
import React from "react"
import { MockBuildInfo, render } from "../../test_helpers"
import { Footer, Language } from "./Footer"

describe("Footer", () => {
  it("renders content", async () => {
    // When
    render(<Footer />)

    // Then
    await screen.findByText("Copyright", { exact: false })
    await screen.findByText(Language.buildInfoText(MockBuildInfo))
  })
})

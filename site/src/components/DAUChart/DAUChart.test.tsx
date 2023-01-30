import { render } from "testHelpers/renderHelpers"
import { DAUChart, Language } from "./DAUChart"

import { screen } from "@testing-library/react"
import { ResizeObserver } from "resize-observer"

// The Chart performs dynamic resizes which fail in tests without this.
Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
})

describe("DAUChart", () => {
  it("renders a helpful paragraph on empty state", async () => {
    render(
      <DAUChart
        daus={{
          entries: [],
        }}
      />,
    )

    await screen.findAllByText(Language.loadingText)
  })
  it("renders a graph", async () => {
    render(
      <DAUChart
        daus={{
          entries: [{ date: "2020-01-01", amount: 1 }],
        }}
      />,
    )

    await screen.findAllByText(Language.chartTitle)
  })
})

import { render } from "testHelpers/renderHelpers"
import { DAUChart, Language } from "./DAUCharts"

import { screen } from "@testing-library/react"
import { ResizeObserver } from "resize-observer"

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
})

describe("DAUChart", () => {
  it("renders a helpful paragraph on empty state", async () => {
    render(
      <DAUChart
        templateMetricsData={{
          entries: [],
        }}
      />,
    )

    await screen.findAllByText(Language.loadingText)
  })
  it("renders a graph", async () => {
    render(
      <DAUChart
        templateMetricsData={{
          entries: [{ date: "2020-01-01", daus: 1 }],
        }}
      />,
    )

    await screen.findAllByText(Language.chartTitle)
  })
})

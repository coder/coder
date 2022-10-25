import { screen } from "@testing-library/react"
import { render } from "../../testHelpers/renderHelpers"
import { PaginationWidget } from "./PaginationWidget"
import { createPaginationRef } from "./utils"

describe("PaginatedList", () => {
  it("displays an accessible previous and next button regardless of the number of pages", async () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        paginationRef={createPaginationRef({ page: 1, limit: 25 })}
      />,
    )

    expect(
      await screen.findByRole("button", { name: "Previous page" }),
    ).toBeTruthy()
    expect(
      await screen.findByRole("button", { name: "Next page" }),
    ).toBeTruthy()
    // Shouldn't render any pages if no records are passed in
    expect(
      await container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(0)
  })

  it("displays the expected number of pages with one ellipsis tile", async () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        numRecords={200}
        paginationRef={createPaginationRef({ page: 1, limit: 12 })}
      />,
    )

    // 7 total spaces. 6 are page numbers, one is ellipsis
    expect(
      await container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(6)
  })

  it("displays the expected number of pages with two ellipsis tiles", async () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        numRecords={200}
        paginationRef={createPaginationRef({ page: 6, limit: 12 })}
      />,
    )

    // 7 total spaces. 2 sets of ellipsis on either side of the active page
    expect(
      await container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(5)
  })
})

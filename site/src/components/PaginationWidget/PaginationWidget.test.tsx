import { screen } from "@testing-library/react"
import { render } from "../../testHelpers/renderHelpers"
import { PaginationWidget } from "./PaginationWidget"

describe("PaginatedList", () => {
  it("displays an accessible previous and next button regardless of the number of pages", async () => {
    render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => alert("Previous click")}
        onNextClick={() => alert("Next click")}
      />,
    )

    expect(await screen.findByRole("button", { name: "Previous page" })).toBeTruthy()
    // expect(screen.findByText('[aria-label="Previous page"]')).toBeTruthy()
    expect(await screen.findByRole("button", { name: "Next page" })).toBeTruthy()

    // expect(screen.findByText('[aria-label="Next page"]')).toBeTruthy()
    // Shouldn't render any pages if no records are passed in
    expect(await screen.findByRole("button", { name: "Page button" })).toBeUndefined()

    // expect(screen.findByText('[name="Page button"]')).toHaveLength(0)
  })

  it("displays the expected number of pages", async () => {
    render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => alert("Previous click")}
        onNextClick={() => alert("Next click")}
        onPageClick={(page) => alert(`Page ${page} clicked`)}
        numRecords={200}
        numRecordsPerPage={12}
        activePage={1}
      />,
    )

    // 7 total spaces. 6 are page numbers, one is ellipsis
    // expect(screen.findByText('[name="Page button"]')).toHaveLength(6)
    expect(await screen.findByRole("button", { name: "Page button" })).toHaveLength(6)

    render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => alert("Previous click")}
        onNextClick={() => alert("Next click")}
        onPageClick={(page) => alert(`Page ${page} clicked`)}
        numRecords={200}
        numRecordsPerPage={12}
        activePage={6}
      />,
    )

    // 7 total spaces. 2 sets of ellipsis on either side of the active page
    // expect(screen.findByText('[name="Page button"]')).toHaveLength(5)
    expect(await screen.findByRole("button", { name: "Page button" })).toHaveLength(5)
  })
})

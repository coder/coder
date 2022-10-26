import { screen } from "@testing-library/react"
import { render } from "../../testHelpers/renderHelpers"
import { PaginationWidget } from "./PaginationWidget"

describe("PaginatedList", () => {
  it("displays an accessible previous and next button", () => {
    render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        numRecords={200}
        numRecordsPerPage={12}
        activePage={1}
        onPrevClick={() => jest.fn()}
        onNextClick={() => jest.fn()}
      />,
    )

    expect(
      screen.getByRole("button", { name: "Previous page" }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Next page" }),
    ).toBeInTheDocument()
  })

  it("displays the expected number of pages with one ellipsis tile", () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => jest.fn()}
        onNextClick={() => jest.fn()}
        onPageClick={(_) => jest.fn()}
        numRecords={200}
        numRecordsPerPage={12}
        activePage={1}
      />,
    )

    // 7 total spaces. 6 are page numbers, one is ellipsis
    expect(
      container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(6)
  })

  it("displays the expected number of pages with two ellipsis tiles", () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => jest.fn()}
        onNextClick={() => jest.fn()}
        onPageClick={(_) => jest.fn()}
        numRecords={200}
        numRecordsPerPage={12}
        activePage={6}
      />,
    )

    // 7 total spaces. 2 sets of ellipsis on either side of the active page
    expect(
      container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(5)
  })
})

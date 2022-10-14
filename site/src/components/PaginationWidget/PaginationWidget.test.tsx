import { screen } from "@testing-library/react"
import { render } from "../../testHelpers/renderHelpers"
import { PaginationWidget } from "./PaginationWidget"

describe("PaginatedList", () => {
  it("displays an accessible previous and next button regardless of the number of pages", async () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => jest.fn()}
        onNextClick={() => jest.fn()}
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
      await container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(6)
  })

  it("displays the expected number of pages with two ellipsis tiles", async () => {
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
      await container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(5)
  })
})

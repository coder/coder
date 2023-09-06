import { screen } from "@testing-library/react";
import { render } from "../../testHelpers/renderHelpers";
import { PaginationWidget } from "./PaginationWidget";
import { createPaginationRef } from "./utils";

describe("PaginatedList", () => {
  it("displays an accessible previous and next button", () => {
    render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        paginationRef={createPaginationRef({ page: 2, limit: 12 })}
        numRecords={200}
      />,
    );

    expect(screen.getByRole("button", { name: "Previous page" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled();
  });

  it("displays the expected number of pages with one ellipsis tile", () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        numRecords={200}
        paginationRef={createPaginationRef({ page: 1, limit: 12 })}
      />,
    );

    // 7 total spaces. 6 are page numbers, one is ellipsis
    expect(
      container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(6);
  });

  it("displays the expected number of pages with two ellipsis tiles", () => {
    const { container } = render(
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        numRecords={200}
        paginationRef={createPaginationRef({ page: 6, limit: 12 })}
      />,
    );

    // 7 total spaces. 2 sets of ellipsis on either side of the active page
    expect(
      container.querySelectorAll(`button[name="Page button"]`),
    ).toHaveLength(5);
  });

  it("disables the previous button on the first page", () => {
    render(
      <PaginationWidget
        numRecords={100}
        paginationRef={createPaginationRef({ page: 1, limit: 25 })}
      />,
    );
    const prevButton = screen.getByLabelText("Previous page");
    expect(prevButton).toBeDisabled();
  });

  it("disables the next button on the last page", () => {
    render(
      <PaginationWidget
        numRecords={100}
        paginationRef={createPaginationRef({ page: 4, limit: 25 })}
      />,
    );
    const nextButton = screen.getByLabelText("Next page");
    expect(nextButton).toBeDisabled();
  });
});

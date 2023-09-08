import { screen } from "@testing-library/react";
import { render } from "testHelpers/renderHelpers";
import { PaginationWidget } from "./PaginationWidget";
import { createPaginationRef } from "./utils";

describe("PaginatedList", () => {
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

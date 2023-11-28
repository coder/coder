import { type ComponentProps, type HTMLAttributes } from "react";
import { Pagination, type PaginationResult } from "./Pagination";

import { renderComponent } from "testHelpers/renderHelpers";
import { waitFor } from "@testing-library/react";

beforeAll(() => {
  jest.useFakeTimers();
});

afterAll(() => {
  jest.clearAllMocks();
  jest.useRealTimers();
});

type ResultBase = Omit<
  PaginationResult,
  "isPreviousData" | "currentChunk" | "totalRecords" | "totalPages"
>;

const mockPaginationResult: ResultBase = {
  isSuccess: false,
  currentPage: 1,
  limit: 25,
  hasNextPage: false,
  hasPreviousPage: false,
  goToPreviousPage: () => {},
  goToNextPage: () => {},
  goToFirstPage: () => {},
  onPageChange: () => {},
};

const initialRenderResult: PaginationResult = {
  ...mockPaginationResult,
  isSuccess: false,
  isPreviousData: false,
  currentChunk: undefined,
  hasNextPage: false,
  hasPreviousPage: false,
  totalRecords: undefined,
  totalPages: undefined,
};

const successResult: PaginationResult = {
  ...mockPaginationResult,
  isSuccess: true,
  isPreviousData: false,
  currentChunk: 1,
  totalPages: 1,
  totalRecords: 4,
};

type TestProps = Omit<
  ComponentProps<typeof Pagination>,
  keyof HTMLAttributes<HTMLDivElement>
>;

const mockUnitLabel = "ducks";

function render2(props: TestProps) {
  return renderComponent(<Pagination {...props} />);
}

/**
 * Expected state transitions:
 *
 * 1. Initial render - isPreviousData is false, while currentPage can be any
 *    number (but will usually be 1)
 *    1. Re-render from first-ever page loading in - currentPage stays the same,
 *       while isPreviousData stays false (data changes elsewhere in the app,
 *       though)
 * 2. Re-render from user changing the page - currentPage becomes the new page,
 *    while isPreviousData depends on cache state
 *    1. Change to page that's already been fetched - isPreviousData is false
 *    2. Change to new page - isPreviousData is true during the transition
 * 3. Re-render fetch for new page succeeding - currentPage stays the same, but
 *    isPreviousData flips from true to false
 */
describe(`${Pagination.name}`, () => {
  describe("Initial render", () => {
    it("Does absolutely nothing - no calls to any scrolls", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");

      render2({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: initialRenderResult,
      });

      setTimeout(() => {
        expect(mockScroll).not.toBeCalled();
      }, 5000);

      await jest.runAllTimersAsync();
    });
  });

  describe("Responding to changes in isPreviousData (showing data for previous page while new page is loading)", () => {
    // This should be impossible, but testing it just to be on the safe side
    it("Does nothing when isPreviousData flips from false to true while currentPage stays the same", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");

      const { rerender } = render2({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: initialRenderResult,
      });

      rerender(
        <Pagination
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{ ...successResult, isPreviousData: true }}
        />,
      );

      setTimeout(() => {
        expect(mockScroll).not.toBeCalled();
      }, 5000);

      await jest.runAllTimersAsync();
    });

    it("Triggers scroll if scroll has been queued while waiting for isPreviousData to flip from true to false", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");

      const { rerender } = render2({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: successResult,
      });

      setTimeout(() => {
        expect(mockScroll).not.toBeCalled();
      }, 5000);

      await jest.runAllTimersAsync();

      rerender(
        <Pagination
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...successResult,
            currentPage: 2,
            isPreviousData: true,
          }}
        />,
      );

      rerender(
        <Pagination
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...successResult,
            currentPage: 2,
            isPreviousData: false,
          }}
        />,
      );

      await waitFor(() => expect(mockScroll).toBeCalled());
    });

    it.skip("Does nothing if scroll is canceled by the time isPreviousData flips from true to false", async () => {
      expect.hasAssertions();
    });
  });

  describe("Responding to page changes", () => {
    it.skip("Triggers scroll immediately if data is cached", async () => {
      expect.hasAssertions();
    });

    it.skip("Queues up a scroll if new page data's needs to be fetched (cache miss)", async () => {
      expect.hasAssertions();
    });
  });
});

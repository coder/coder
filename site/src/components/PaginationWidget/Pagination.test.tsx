import { type ComponentProps, type HTMLAttributes } from "react";
import { Pagination, type PaginationResult } from "./Pagination";

import { renderComponent } from "testHelpers/renderHelpers";
import { fireEvent, waitFor } from "@testing-library/react";

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

function render(props: TestProps) {
  return renderComponent(<Pagination {...props} />);
}

function assertNoScroll(mockScroll: jest.SpyInstance) {
  setTimeout(() => {
    expect(mockScroll).not.toBeCalled();
  }, 5000);

  return jest.runAllTimersAsync();
}

async function mountWithSuccess(mockScroll: jest.SpyInstance) {
  // eslint-disable-next-line testing-library/render-result-naming-convention -- Forced destructuring just makes this awkward
  const result = render({
    paginationUnitLabel: mockUnitLabel,
    paginationResult: successResult,
  });

  await assertNoScroll(mockScroll);
  return result;
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

      render({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: initialRenderResult,
      });

      await assertNoScroll(mockScroll);
    });
  });

  describe("Responding to page changes", () => {
    it("Triggers scroll immediately if currentPage changes and isPreviousData is immediately false (previous query is cached)", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");
      const { rerender } = await mountWithSuccess(mockScroll);

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

    it("Does nothing observable if page changes and isPreviousData is true (scroll will get queued, but will not be processed)", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");
      const { rerender } = await mountWithSuccess(mockScroll);

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

      await assertNoScroll(mockScroll);
    });
  });

  describe("Responding to changes in React Query's isPreviousData", () => {
    // This should be impossible, but testing it just to be on the safe side
    it("Does nothing when isPreviousData flips from false to true while currentPage stays the same", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");

      const { rerender } = render({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: initialRenderResult,
      });

      rerender(
        <Pagination
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{ ...successResult, isPreviousData: true }}
        />,
      );

      await assertNoScroll(mockScroll);
    });

    it("Triggers scroll if scroll has been queued while waiting for isPreviousData to flip from true to false", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");
      const { rerender } = await mountWithSuccess(mockScroll);

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

    it("Does nothing if scroll is canceled by the time isPreviousData flips from true to false", async () => {
      const mockScroll = jest.spyOn(window, "scrollTo");
      const { rerender } = await mountWithSuccess(mockScroll);

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

      fireEvent.click(window);

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

      await assertNoScroll(mockScroll);
    });
  });
});

import { renderComponent } from "testHelpers/renderHelpers";
import { fireEvent, waitFor } from "@testing-library/react";
import type { PropsWithChildren, ComponentProps, HTMLAttributes } from "react";

import { PaginationContainer } from "./PaginationContainer";
import {
  mockInitialRenderResult,
  mockSuccessResult,
} from "./PaginationContainer.mocks";

// Test environments don't implement scrollIntoView for some reason; have to
// add an implementation so that things don't error out
const prevScrollIntoView = window.HTMLElement.prototype.scrollIntoView;

beforeAll(() => {
  window.HTMLElement.prototype.scrollIntoView = jest.fn();
  jest.useFakeTimers();
});

afterAll(() => {
  window.HTMLElement.prototype.scrollIntoView = prevScrollIntoView;
  jest.useRealTimers();
});

type TestProps = Omit<
  ComponentProps<typeof PaginationContainer>,
  keyof HTMLAttributes<HTMLDivElement>
> &
  PropsWithChildren;

const mockUnitLabel = "ducks";

function render(props: TestProps) {
  return renderComponent(<PaginationContainer {...props} />);
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
    paginationResult: mockSuccessResult,
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
describe(`${PaginationContainer.name}`, () => {
  describe("Initial render", () => {
    it("Does absolutely nothing - should not scroll on component mount because that will violently hijack the user's browser", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");

      render({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: mockInitialRenderResult,
      });

      await assertNoScroll(mockScroll);
    });
  });

  describe("Responding to page changes", () => {
    it("Triggers scroll immediately if currentPage changes and isPreviousData is immediately false (previous query is cached)", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");
      const { rerender } = await mountWithSuccess(mockScroll);

      rerender(
        <PaginationContainer
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...mockSuccessResult,
            currentPage: 2,
            isPreviousData: false,
          }}
        />,
      );

      await waitFor(() => expect(mockScroll).toBeCalled());
    });

    it("Does nothing observable if page changes and isPreviousData is true (scroll will get queued, but will not be processed)", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");
      const { rerender } = await mountWithSuccess(mockScroll);

      rerender(
        <PaginationContainer
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...mockSuccessResult,
            currentPage: 2,
            isPreviousData: true,
          }}
        />,
      );

      await assertNoScroll(mockScroll);
    });
  });

  describe("Responding to changes in React Query's isPreviousData", () => {
    it("Does nothing when isPreviousData flips from false to true while currentPage stays the same (safety net for 'impossible' case)", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");

      const { rerender } = render({
        paginationUnitLabel: mockUnitLabel,
        paginationResult: mockInitialRenderResult,
      });

      rerender(
        <PaginationContainer
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{ ...mockSuccessResult, isPreviousData: true }}
        />,
      );

      await assertNoScroll(mockScroll);
    });

    it("Triggers scroll if scroll has been queued while waiting for isPreviousData to flip from true to false", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");
      const { rerender } = await mountWithSuccess(mockScroll);

      rerender(
        <PaginationContainer
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...mockSuccessResult,
            currentPage: 2,
            isPreviousData: true,
          }}
        />,
      );

      rerender(
        <PaginationContainer
          paginationUnitLabel={mockUnitLabel}
          paginationResult={{
            ...mockSuccessResult,
            currentPage: 2,
            isPreviousData: false,
          }}
        />,
      );

      await waitFor(() => expect(mockScroll).toBeCalled());
    });

    it("Cancels a scroll if user interacts with the browser in any way before isPreviousData flips from true to false", async () => {
      const mockScroll = jest.spyOn(window, "scrollBy");

      // Values are based on (keyof WindowEventMap), but frustratingly, the
      // native events aren't camel-case, while the fireEvent properties are
      const userInteractionEvents = [
        "click",
        "scroll",
        "pointerEnter",
        "touchStart",
        "keyDown",
      ] as const;

      for (const event of userInteractionEvents) {
        const { rerender, unmount } = await mountWithSuccess(mockScroll);

        rerender(
          <PaginationContainer
            paginationUnitLabel={mockUnitLabel}
            paginationResult={{
              ...mockSuccessResult,
              currentPage: 2,
              isPreviousData: true,
            }}
          />,
        );

        fireEvent[event](window);

        rerender(
          <PaginationContainer
            paginationUnitLabel={mockUnitLabel}
            paginationResult={{
              ...mockSuccessResult,
              currentPage: 2,
              isPreviousData: false,
            }}
          />,
        );

        await assertNoScroll(mockScroll);
        unmount();
      }
    });
  });
});

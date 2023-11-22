import { renderHookWithAuth } from "testHelpers/renderHelpers";
import { waitFor } from "@testing-library/react";

import {
  type PaginatedData,
  type UsePaginatedQueryOptions,
  usePaginatedQuery,
} from "./usePaginatedQuery";

beforeAll(() => {
  jest.useFakeTimers();
});

afterAll(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
});

function render<
  TQueryFnData extends PaginatedData = PaginatedData,
  TQueryPayload = never,
>(
  queryOptions: UsePaginatedQueryOptions<TQueryFnData, TQueryPayload>,
  route?: `/?page=${string}`,
) {
  type Props = { options: typeof queryOptions };

  return renderHookWithAuth(
    ({ options }: Props) => usePaginatedQuery(options),
    {
      route,
      path: "/",
      initialProps: {
        options: queryOptions,
      },
    },
  );
}

/**
 * There are a lot of test cases in this file. Scoping mocking to inner describe
 * function calls to limit cognitive load of maintaining this file.
 */
describe(`${usePaginatedQuery.name} - Overall functionality`, () => {
  describe("queryPayload method", () => {
    const mockQueryFn = jest.fn(() => Promise.resolve({ count: 0 }));

    it("Passes along an undefined payload if queryPayload is not used", async () => {
      const mockQueryKey = jest.fn(() => ["mockQuery"]);

      await render({
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      const payloadValueMock = expect.objectContaining({
        payload: undefined,
      });

      expect(mockQueryKey).toHaveBeenCalledWith(payloadValueMock);
      expect(mockQueryFn).toHaveBeenCalledWith(payloadValueMock);
    });

    it("Passes along type-safe payload if queryPayload is provided", async () => {
      const mockQueryKey = jest.fn(({ payload }) => {
        return ["mockQuery", payload];
      });

      const testPayloadValues = [1, "Blah", { cool: true }];
      for (const payload of testPayloadValues) {
        const { unmount } = await render({
          queryPayload: () => payload,
          queryKey: mockQueryKey,
          queryFn: mockQueryFn,
        });

        const matcher = expect.objectContaining({ payload });
        expect(mockQueryKey).toHaveBeenCalledWith(matcher);
        expect(mockQueryFn).toHaveBeenCalledWith(matcher);
        unmount();
      }
    });
  });

  describe("Querying for current page", () => {
    const mockQueryKey = jest.fn(() => ["mock"]);
    const mockQueryFn = jest.fn(() => Promise.resolve({ count: 50 }));

    it("Parses page number if it exists in URL params", async () => {
      const pageNumbers = [1, 2, 7, 39, 743];

      for (const num of pageNumbers) {
        const { result, unmount } = await render(
          { queryKey: mockQueryKey, queryFn: mockQueryFn },
          `/?page=${num}`,
        );

        expect(result.current.currentPage).toBe(num);
        unmount();
      }
    });

    it("Defaults to page 1 if no page value can be parsed from params", async () => {
      const { result } = await render({
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      expect(result.current.currentPage).toBe(1);
    });
  });

  describe("Prefetching", () => {
    const mockQueryKey = jest.fn(({ pageNumber }) => ["query", pageNumber]);
    const mockQueryFn = jest.fn(({ pageNumber, limit }) => {
      return Promise.resolve({
        data: new Array(limit).fill(pageNumber),
        count: 75,
      });
    });

    const testPrefetch = async (
      startingPage: number,
      targetPage: number,
      shouldMatch: boolean,
    ) => {
      await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        `/?page=${startingPage}`,
      );

      const pageMatcher = expect.objectContaining({ pageNumber: targetPage });
      if (shouldMatch) {
        await waitFor(() => expect(mockQueryFn).toBeCalledWith(pageMatcher));
      } else {
        // Can't use waitFor to test this, because the expect call will
        // immediately succeed for the not case, even though queryFn needs to be
        // called async via React Query
        setTimeout(() => {
          expect(mockQueryFn).not.toBeCalledWith(pageMatcher);
        }, 1000);

        jest.runAllTimers();
      }
    };

    it("Prefetches the previous page if it exists", async () => {
      await testPrefetch(2, 1, true);
    });

    it.skip("Prefetches the next page if it exists", async () => {
      await testPrefetch(2, 3, true);
    });

    it.skip("Avoids prefetch for previous page if it doesn't exist", async () => {
      await testPrefetch(1, 0, false);
      await testPrefetch(6, 5, false);
    });

    it("Avoids prefetch for next page if it doesn't exist", async () => {
      await testPrefetch(3, 4, false);
    });

    it.skip("Reuses the same queryKey and queryFn methods for the current page and all prefetching (on a given render)", async () => {
      const startPage = 2;
      await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        `/?page=${startPage}`,
      );

      const currentMatcher = expect.objectContaining({ pageNumber: startPage });
      expect(mockQueryKey).toBeCalledWith(currentMatcher);
      expect(mockQueryFn).toBeCalledWith(currentMatcher);

      const prevPageMatcher = expect.objectContaining({
        pageNumber: startPage - 1,
      });
      const nextPageMatcher = expect.objectContaining({
        pageNumber: startPage + 1,
      });

      await waitFor(() => expect(mockQueryKey).toBeCalledWith(prevPageMatcher));
      await waitFor(() => expect(mockQueryFn).toBeCalledWith(prevPageMatcher));
      await waitFor(() => expect(mockQueryKey).toBeCalledWith(nextPageMatcher));
      await waitFor(() => expect(mockQueryFn).toBeCalledWith(nextPageMatcher));
    });
  });

  describe("Invalid page safety nets/redirects", () => {
    const mockQueryKey = jest.fn(() => ["mock"]);
    const mockQueryFn = jest.fn(({ pageNumber, limit }) =>
      Promise.resolve({
        data: new Array(limit).fill(pageNumber),
        count: 100,
      }),
    );

    it("Immediately/synchronously defaults to page 1 if params are corrupt/invalid", async () => {
      const { result } = await render(
        {
          queryKey: mockQueryKey,
          queryFn: mockQueryFn,
        },
        "/?page=Cat",
      );

      expect(result.current.currentPage).toBe(1);
    });

    it("Auto-redirects user to last page if requested page overshoots total pages", async () => {
      const { result } = await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        "/?page=35",
      );

      await waitFor(() => expect(result.current.currentPage).toBe(4));
    });

    it("Auto-redirects user to first page if requested page goes below 1", async () => {
      const { result } = await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        "/?page=-9999",
      );

      await waitFor(() => expect(result.current.currentPage).toBe(1));
    });

    it.skip("Calls the custom onInvalidPageChange callback if provided", async () => {
      expect.hasAssertions();
    });
  });

  describe("Passing outside value for URLSearchParams", () => {
    it.skip("Reads from searchParams property if provided", async () => {
      expect.hasAssertions();
    });

    it.skip("Flushes state changes via provided searchParams property", async () => {
      expect.hasAssertions();
    });
  });
});

describe(`${usePaginatedQuery.name} - Returned properties`, () => {
  describe("Conditional render output", () => {
    it.skip("Always has select properties be defined regardless of fetch status", async () => {
      expect.hasAssertions();
    });

    it.skip("Flips other properties to be defined after on-mount fetch succeeds", async () => {
      expect.hasAssertions();
    });
  });

  describe("Page change methods", () => {
    test.skip("goToFirstPage always succeeds regardless of fetch status", async () => {
      expect.hasAssertions();
    });

    test.skip("goToNextPage works only if hasNextPage is true", async () => {
      expect.hasAssertions();
    });

    test.skip("goToPreviousPage works only if hasPreviousPage is true", async () => {
      expect.hasAssertions();
    });

    test.skip("onPageChange cleans 'corrupt' numeric values before navigating", async () => {
      expect.hasAssertions();
    });

    test.skip("onPageChange rejects impossible numeric values and does nothing", async () => {
      expect.hasAssertions();
    });
  });
});

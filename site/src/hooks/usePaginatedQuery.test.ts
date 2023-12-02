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
  options: UsePaginatedQueryOptions<TQueryFnData, TQueryPayload>,
  route?: `/?page=${string}`,
) {
  return renderHookWithAuth(({ options }) => usePaginatedQuery(options), {
    route,
    path: "/",
    initialProps: { options },
  });
}

/**
 * There are a lot of test cases in this file. Scoping mocking to inner describe
 * function calls to limit the cognitive load of maintaining all this stuff
 */
describe(usePaginatedQuery.name, () => {
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

    type Context = { pageNumber: number; limit: number };
    const mockQueryFnImplementation = ({ pageNumber, limit }: Context) => {
      const data: { value: number }[] = [];
      if (pageNumber * limit < 75) {
        for (let i = 0; i < limit; i++) {
          data.push({ value: i });
        }
      }

      return Promise.resolve({ data, count: 75 });
    };

    const testPrefetch = async (
      startingPage: number,
      targetPage: number,
      shouldMatch: boolean,
    ) => {
      // Have to reinitialize mock function every call to avoid false positives
      // from shared mutable tracking state
      const mockQueryFn = jest.fn(mockQueryFnImplementation);
      const { result } = await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        `/?page=${startingPage}`,
      );

      const pageMatcher = expect.objectContaining({ pageNumber: targetPage });
      if (shouldMatch) {
        await waitFor(() => expect(result.current.totalRecords).toBeDefined());
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

    it("Prefetches the next page if it exists", async () => {
      await testPrefetch(2, 3, true);
    });

    it("Avoids prefetch for previous page if it doesn't exist", async () => {
      await testPrefetch(1, 0, false);
      await testPrefetch(6, 5, false);
    });

    it("Avoids prefetch for next page if it doesn't exist", async () => {
      await testPrefetch(3, 4, false);
    });

    it("Reuses the same queryKey and queryFn methods for the current page and all prefetching (on a given render)", async () => {
      const startPage = 2;
      const mockQueryFn = jest.fn(mockQueryFnImplementation);

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
      await waitFor(() => expect(mockQueryKey).toBeCalledWith(prevPageMatcher));
      await waitFor(() => expect(mockQueryFn).toBeCalledWith(prevPageMatcher));

      const nextPageMatcher = expect.objectContaining({
        pageNumber: startPage + 1,
      });
      await waitFor(() => expect(mockQueryKey).toBeCalledWith(nextPageMatcher));
      await waitFor(() => expect(mockQueryFn).toBeCalledWith(nextPageMatcher));
    });
  });

  describe("Safety nets/redirects for invalid pages", () => {
    const mockQueryKey = jest.fn(() => ["mock"]);
    const mockQueryFn = jest.fn(({ pageNumber, limit }) =>
      Promise.resolve({
        data: new Array(limit).fill(pageNumber),
        count: 100,
      }),
    );

    it("No custom callback: synchronously defaults to page 1 if params are corrupt/invalid", async () => {
      const { result } = await render(
        {
          queryKey: mockQueryKey,
          queryFn: mockQueryFn,
        },
        "/?page=Cat",
      );

      expect(result.current.currentPage).toBe(1);
    });

    it("No custom callback: auto-redirects user to last page if requested page overshoots total pages", async () => {
      const { result } = await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        "/?page=35",
      );

      await waitFor(() => expect(result.current.currentPage).toBe(4));
    });

    it("No custom callback: auto-redirects user to first page if requested page goes below 1", async () => {
      const { result } = await render(
        { queryKey: mockQueryKey, queryFn: mockQueryFn },
        "/?page=-9999",
      );

      await waitFor(() => expect(result.current.currentPage).toBe(1));
    });

    it("With custom callback: Calls callback and does not update search params automatically", async () => {
      const testControl = new URLSearchParams({
        page: "1000",
      });

      const onInvalidPageChange = jest.fn();
      await render({
        onInvalidPageChange,
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
        searchParams: testControl,
      });

      await waitFor(() => {
        expect(onInvalidPageChange).toBeCalledWith(
          expect.objectContaining({
            pageNumber: expect.any(Number),
            limit: expect.any(Number),
            offset: expect.any(Number),
            totalPages: expect.any(Number),
            searchParams: expect.any(URLSearchParams),
            setSearchParams: expect.any(Function),
          }),
        );
      });

      expect(testControl.get("page")).toBe("1000");
    });
  });

  describe("Passing in searchParams property", () => {
    const mockQueryKey = jest.fn(() => ["mock"]);
    const mockQueryFn = jest.fn(({ pageNumber, limit }) =>
      Promise.resolve({
        data: new Array(limit).fill(pageNumber),
        count: 100,
      }),
    );

    it("Reads from searchParams property if provided", async () => {
      const searchParams = new URLSearchParams({
        page: "2",
      });

      const { result } = await render({
        searchParams,
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      expect(result.current.currentPage).toBe(2);
    });

    it("Flushes state changes via provided searchParams property instead of internal searchParams", async () => {
      const searchParams = new URLSearchParams({
        page: "2",
      });

      const { result } = await render({
        searchParams,
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      result.current.goToFirstPage();
      expect(searchParams.get("page")).toBe("1");
    });
  });
});

describe(`${usePaginatedQuery.name} - Returned properties`, () => {
  describe("Page change methods", () => {
    const mockQueryKey = jest.fn(() => ["mock"]);

    const mockQueryFn = jest.fn(({ pageNumber, limit }) => {
      type Data = PaginatedData & { data: readonly number[] };

      return new Promise<Data>((resolve) => {
        setTimeout(() => {
          resolve({
            data: new Array(limit).fill(pageNumber),
            count: 100,
          });
        }, 10_000);
      });
    });

    test("goToFirstPage always succeeds regardless of fetch status", async () => {
      const queryFns = [mockQueryFn, jest.fn(() => Promise.reject("Too bad"))];

      for (const queryFn of queryFns) {
        const { result, unmount } = await render(
          { queryFn, queryKey: mockQueryKey },
          "/?page=5",
        );

        expect(result.current.currentPage).toBe(5);
        result.current.goToFirstPage();
        await waitFor(() => expect(result.current.currentPage).toBe(1));
        unmount();
      }
    });

    test("goToNextPage works only if hasNextPage is true", async () => {
      const { result } = await render(
        {
          queryKey: mockQueryKey,
          queryFn: mockQueryFn,
        },
        "/?page=1",
      );

      expect(result.current.hasNextPage).toBe(false);
      result.current.goToNextPage();
      expect(result.current.currentPage).toBe(1);

      await jest.runAllTimersAsync();
      await waitFor(() => expect(result.current.hasNextPage).toBe(true));
      result.current.goToNextPage();
      await waitFor(() => expect(result.current.currentPage).toBe(2));
    });

    test("goToPreviousPage works only if hasPreviousPage is true", async () => {
      const { result } = await render(
        {
          queryKey: mockQueryKey,
          queryFn: mockQueryFn,
        },
        "/?page=3",
      );

      expect(result.current.hasPreviousPage).toBe(false);
      result.current.goToPreviousPage();
      expect(result.current.currentPage).toBe(3);

      await jest.runAllTimersAsync();
      await waitFor(() => expect(result.current.hasPreviousPage).toBe(true));
      result.current.goToPreviousPage();
      await waitFor(() => expect(result.current.currentPage).toBe(2));
    });

    test("onPageChange accounts for floats and truncates numeric values before navigating", async () => {
      const { result } = await render({
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      await jest.runAllTimersAsync();
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      result.current.onPageChange(2.5);

      await waitFor(() => expect(result.current.currentPage).toBe(2));
    });

    test("onPageChange rejects impossible numeric values and does nothing", async () => {
      const { result } = await render({
        queryKey: mockQueryKey,
        queryFn: mockQueryFn,
      });

      await jest.runAllTimersAsync();
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      result.current.onPageChange(NaN);
      result.current.onPageChange(Infinity);
      result.current.onPageChange(-Infinity);

      setTimeout(() => {
        expect(result.current.currentPage).toBe(1);
      }, 1000);

      jest.runAllTimers();
    });
  });
});

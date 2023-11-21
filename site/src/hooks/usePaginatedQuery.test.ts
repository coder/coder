import { renderHookWithAuth } from "testHelpers/renderHelpers";
import {
  type PaginatedData,
  type UsePaginatedQueryOptions,
  usePaginatedQuery,
} from "./usePaginatedQuery";
import { waitFor } from "@testing-library/react";

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
  route?: `/${string}`,
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

describe(usePaginatedQuery.name, () => {
  describe("queryPayload method", () => {
    const mockQueryFn = jest.fn(() => {
      return { count: 0 };
    });

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
        count: 50,
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

    it("Prefetches the next page if it exists", async () => {
      await testPrefetch(1, 2, true);
    });

    it("Avoids prefetch for previous page if it doesn't exist", async () => {
      await testPrefetch(1, 0, false);
    });

    it("Avoids prefetch for next page if it doesn't exist", async () => {
      await testPrefetch(2, 3, false);
    });

    it("Reuses the same queryKey and queryFn methods for the current page and all prefetching", async () => {
      expect.hasAssertions();
    });
  });

  describe("Invalid page safety nets/redirects", () => {
    it("Auto-redirects user to page 1 if params are corrupt/invalid", async () => {
      expect.hasAssertions();
    });

    it("Auto-redirects user to closest page if request page overshoots", async () => {
      expect.hasAssertions();
    });

    it("Auto-redirects user to first page if request page goes below 1", async () => {
      expect.hasAssertions();
    });

    it("Calls the custom onInvalidPageChange callback if provided", async () => {
      expect.hasAssertions();
    });
  });

  describe("Passing outside value for URLSearchParams", () => {
    it("Reads from searchParams property if provided", async () => {
      expect.hasAssertions();
    });

    it("Flushes state changes via provided searchParams property", async () => {
      expect.hasAssertions();
    });
  });

  describe("Returned properties", () => {});
});

// TODO: This test is timing out after upgrade a few Jest dependencies
// and I was not able to figure out why. When running it specifically, I
// can see many act warnings that may can help us to find the issue.
// (Note: This comment was originally written by Bruno, and was relocated by
// me. If you go poking at `git blame`, disabling these tests was not my idea.

import { renderHookWithAuth } from "testHelpers/hooks";
import { waitFor } from "@testing-library/react";
import {
	type PaginatedData,
	type UsePaginatedQueryOptions,
	usePaginatedQuery,
} from "./usePaginatedQuery";

// Removed global fake timers - they will be used locally where needed
afterEach(() => {
	vi.clearAllMocks();
});

function render<
	TQueryFnData extends PaginatedData = PaginatedData,
	TQueryPayload = never,
>(
	options: UsePaginatedQueryOptions<TQueryFnData, TQueryPayload>,
	route?: `/?page=${string}`,
) {
	return renderHookWithAuth(({ options }) => usePaginatedQuery(options), {
		routingOptions: {
			route,
			path: "/",
		},
		renderOptions: {
			initialProps: { options },
		},
	});
}

describe(usePaginatedQuery.name, () => {
	describe("queryPayload method", () => {
		const mockQueryFn = vi.fn(() => Promise.resolve({ count: 0 }));

		it("Passes along an undefined payload if queryPayload is not used", async () => {
			const mockQueryKey = vi.fn(() => ["mockQuery"]);

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
			const mockQueryKey = vi.fn(({ payload }) => {
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
		const mockQueryKey = vi.fn(() => ["mock"]);
		const mockQueryFn = vi.fn(() => Promise.resolve({ count: 50 }));

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
		const mockQueryKey = vi.fn(({ pageNumber }) => ["query", pageNumber]);

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
			const mockQueryFn = vi.fn(mockQueryFnImplementation);
			const { result } = await render(
				{ queryKey: mockQueryKey, queryFn: mockQueryFn },
				`/?page=${startingPage}`,
			);

			const pageMatcher = expect.objectContaining({ pageNumber: targetPage });
			if (shouldMatch) {
				await waitFor(() => expect(result.current.totalRecords).toBeDefined());
				await waitFor(() => expect(mockQueryFn).toBeCalledWith(pageMatcher));
			} else {
				// Use fake timers only for this specific case
				vi.useFakeTimers();
				// Can't use waitFor to test this, because the expect call will
				// immediately succeed for the not case, even though queryFn needs to be
				// called async via React Query
				setTimeout(() => {
					expect(mockQueryFn).not.toBeCalledWith(pageMatcher);
				}, 1000);

				vi.runAllTimers();
				vi.useRealTimers();
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
			const mockQueryFn = vi.fn(mockQueryFnImplementation);

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
		const mockQueryKey = vi.fn(() => ["mock"]);
		const mockQueryFn = vi.fn(({ pageNumber, limit }) =>
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

			const onInvalidPageChange = vi.fn();
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
		const mockQueryKey = vi.fn(() => ["mock"]);
		const mockQueryFn = vi.fn(({ pageNumber, limit }) =>
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
		const mockQueryKey = vi.fn(() => ["mock"]);

		const mockQueryFn = vi.fn(({ pageNumber, limit }) => {
			type Data = PaginatedData & { data: readonly number[] };

			return Promise.resolve({
				data: new Array(limit).fill(pageNumber),
				count: 100,
			});
		});

		test("goToFirstPage always succeeds regardless of fetch status", async () => {
			const queryFns = [mockQueryFn, vi.fn(() => Promise.reject("Too bad"))];

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

			// Wait for the query to complete and check we have next pages
			await waitFor(() => expect(result.current.isSuccess).toBe(true));
			expect(result.current.hasNextPage).toBe(true);

			// Can go to next page when hasNextPage is true
			result.current.goToNextPage();
			await waitFor(() => expect(result.current.currentPage).toBe(2));

			// Navigate to last page (page 4, since we have 100 items with 25 per page)
			result.current.onPageChange(4);
			await waitFor(() => expect(result.current.currentPage).toBe(4));

			// Now hasNextPage should be false and goToNextPage should not change the page
			expect(result.current.hasNextPage).toBe(false);
			result.current.goToNextPage();
			expect(result.current.currentPage).toBe(4);
		});

		test("goToPreviousPage works only if hasPreviousPage is true", async () => {
			const { result } = await render(
				{
					queryKey: mockQueryKey,
					queryFn: mockQueryFn,
				},
				"/?page=3",
			);

			// Wait for the query to complete and check we have previous pages
			await waitFor(() => expect(result.current.isSuccess).toBe(true));
			expect(result.current.hasPreviousPage).toBe(true);

			// Can go to previous page when hasPreviousPage is true
			result.current.goToPreviousPage();
			await waitFor(() => expect(result.current.currentPage).toBe(2));

			// Navigate to first page
			result.current.goToFirstPage();
			await waitFor(() => expect(result.current.currentPage).toBe(1));

			// Now hasPreviousPage should be false and goToPreviousPage should not change the page
			expect(result.current.hasPreviousPage).toBe(false);
			result.current.goToPreviousPage();
			expect(result.current.currentPage).toBe(1);
		});

		test("onPageChange accounts for floats and truncates numeric values before navigating", async () => {
			const { result } = await render({
				queryKey: mockQueryKey,
				queryFn: mockQueryFn,
			});

			// Wait for the initial query to complete
			await waitFor(() => expect(result.current.isSuccess).toBe(true));
			result.current.onPageChange(2.5);

			await waitFor(() => expect(result.current.currentPage).toBe(2));
		});

		test("onPageChange rejects impossible numeric values and does nothing", async () => {
			const { result } = await render({
				queryKey: mockQueryKey,
				queryFn: mockQueryFn,
			});

			// Wait for the initial query to complete
			await waitFor(() => expect(result.current.isSuccess).toBe(true));

			result.current.onPageChange(Number.NaN);
			result.current.onPageChange(Number.POSITIVE_INFINITY);
			result.current.onPageChange(Number.NEGATIVE_INFINITY);

			// Give it a moment to ensure no navigation happens
			await new Promise((resolve) => setTimeout(resolve, 10));
			expect(result.current.currentPage).toBe(1);
		});
	});
});

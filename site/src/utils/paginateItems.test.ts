import { describe, expect, it } from "vitest";
import { paginateItems } from "./paginateItems";

// 25 items numbered 1–25 for readable assertions.
const items = Array.from({ length: 25 }, (_, i) => i + 1);

describe("paginateItems", () => {
	it("returns the first page of items", () => {
		const result = paginateItems(items, 10, 1);
		expect(result.pagedItems).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
		expect(result.clampedPage).toBe(1);
		expect(result.totalPages).toBe(3);
		expect(result.hasPreviousPage).toBe(false);
		expect(result.hasNextPage).toBe(true);
	});

	it("returns a partial last page", () => {
		const result = paginateItems(items, 10, 3);
		expect(result.pagedItems).toEqual([21, 22, 23, 24, 25]);
		expect(result.clampedPage).toBe(3);
		expect(result.totalPages).toBe(3);
		expect(result.hasPreviousPage).toBe(true);
		expect(result.hasNextPage).toBe(false);
	});

	it("clamps currentPage down when beyond total pages", () => {
		const result = paginateItems(items, 10, 99);
		expect(result.clampedPage).toBe(3);
		expect(result.pagedItems).toEqual([21, 22, 23, 24, 25]);
		expect(result.hasPreviousPage).toBe(true);
		expect(result.hasNextPage).toBe(false);
	});

	it("clamps currentPage up when 0", () => {
		const result = paginateItems(items, 10, 0);
		expect(result.clampedPage).toBe(1);
		expect(result.pagedItems).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
		expect(result.hasPreviousPage).toBe(false);
		expect(result.hasNextPage).toBe(true);
	});

	it("clamps currentPage up when negative", () => {
		const result = paginateItems(items, 10, -5);
		expect(result.clampedPage).toBe(1);
		expect(result.pagedItems).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
		expect(result.hasPreviousPage).toBe(false);
		expect(result.hasNextPage).toBe(true);
	});

	it("returns empty pagedItems with clampedPage=1 for an empty array", () => {
		const result = paginateItems([], 10, 1);
		expect(result.pagedItems).toEqual([]);
		expect(result.clampedPage).toBe(1);
		expect(result.totalPages).toBe(1);
		expect(result.hasPreviousPage).toBe(false);
		expect(result.hasNextPage).toBe(false);
	});

	it("reports hasPreviousPage correctly for middle pages", () => {
		const result = paginateItems(items, 10, 2);
		expect(result.hasPreviousPage).toBe(true);
		expect(result.hasNextPage).toBe(true);
	});
});

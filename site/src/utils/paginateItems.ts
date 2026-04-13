export function paginateItems<T>(
	items: readonly T[],
	pageSize: number,
	currentPage: number,
): {
	pagedItems: T[];
	clampedPage: number;
	totalPages: number;
	hasPreviousPage: boolean;
	hasNextPage: boolean;
} {
	const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
	const clampedPage = Math.max(1, Math.min(currentPage, totalPages));
	const pagedItems = items.slice(
		(clampedPage - 1) * pageSize,
		clampedPage * pageSize,
	);
	return {
		pagedItems,
		clampedPage,
		totalPages,
		hasPreviousPage: clampedPage > 1,
		hasNextPage: clampedPage * pageSize < items.length,
	};
}

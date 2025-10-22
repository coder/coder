import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils";

const paginationKey = "page";

type UsePaginationOptions = Readonly<{
	searchParams: URLSearchParams;
	onSearchParamsChange: (newParams: URLSearchParams) => void;
}>;

type UsePaginationResult = Readonly<{
	page: number;
	limit: number;
	offset: number;
	goToPage: (page: number) => void;
}>;

export function usePagination(
	options: UsePaginationOptions,
): UsePaginationResult {
	const { searchParams, onSearchParamsChange } = options;
	const limit = DEFAULT_RECORDS_PER_PAGE;
	const rawPage = Number.parseInt(searchParams.get(paginationKey) || "1", 10);
	const page = Number.isNaN(rawPage) || rawPage <= 0 ? 1 : rawPage;

	return {
		page,
		limit,
		offset: Math.max(0, (page - 1) * limit),
		goToPage: (newPage) => {
			const abortNavigation =
				page === newPage ||
				!Number.isFinite(newPage) ||
				!Number.isInteger(newPage);
			if (abortNavigation) {
				return;
			}

			const copy = new URLSearchParams(searchParams);
			copy.set("page", newPage.toString());
			onSearchParamsChange(copy);
		},
	};
}

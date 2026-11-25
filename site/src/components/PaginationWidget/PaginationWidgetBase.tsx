import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import { NumberedPageButton, PlaceholderPageButton } from "./PageButtons";
import { PaginationNavButton } from "./PaginationNavButton";
import { buildPagedList } from "./utils";

export type PaginationWidgetBaseProps = {
	currentPage: number;
	pageSize: number;
	totalRecords: number;
	onPageChange: (newPage: number) => void;

	hasPreviousPage?: boolean;
	hasNextPage?: boolean;
	/** Override the computed totalPages.
	 * Used when, e.g., the row count is capped and the user navigates beyond
	 * the known range, so totalPages stays at least as high as currentPage. */
	totalPages?: number;
};

export const PaginationWidgetBase: React.FC<PaginationWidgetBaseProps> = ({
	currentPage,
	pageSize,
	totalRecords,
	onPageChange,
	hasPreviousPage,
	hasNextPage,
	totalPages: totalPagesProp,
}) => {
	const totalPages = totalPagesProp ?? Math.ceil(totalRecords / pageSize);

	if (totalPages < 2) {
		return null;
	}

	const currentPageOffset = (currentPage - 1) * pageSize;
	const isPrevDisabled = !(hasPreviousPage ?? currentPage > 1);
	const isNextDisabled = !(
		hasNextPage ?? pageSize + currentPageOffset < totalRecords
	);

	return (
		<nav aria-label="Pagination">
			<ul className="@container flex items-center justify-center px-5 gap-x-1.5">
				<li className="flex">
					<PaginationNavButton
						disabled={isPrevDisabled}
						aria-label="Previous page"
						onClick={() => {
							if (!isPrevDisabled) {
								onPageChange(currentPage - 1);
							}
						}}
					>
						<ChevronLeftIcon />
					</PaginationNavButton>
				</li>

				{/* Show only the current page when the widget is too narrow for the
				    full row of page blocks and both arrows. */}
				<li className="flex @sm:hidden">
					<NumberedPageButton highlighted pageNumber={currentPage} />
				</li>
				<PaginationRow
					currentPage={currentPage}
					totalPages={totalPages}
					onChange={onPageChange}
				/>

				<li className="flex">
					<PaginationNavButton
						disabled={isNextDisabled}
						aria-label="Next page"
						onClick={() => {
							if (!isNextDisabled) {
								onPageChange(currentPage + 1);
							}
						}}
					>
						<ChevronRightIcon />
					</PaginationNavButton>
				</li>
			</ul>
		</nav>
	);
};

type PaginationRowProps = {
	currentPage: number;
	totalPages: number;
	onChange: (newPage: number) => void;
};

const PaginationRow: React.FC<PaginationRowProps> = ({
	currentPage,
	totalPages,
	onChange,
}) => {
	const pageInfo = buildPagedList(totalPages, currentPage);
	const pagesOmitted = totalPages - pageInfo.length - 1;

	return pageInfo.map((pageEntry) => (
		<li key={pageEntry} className="hidden @sm:flex">
			{pageEntry === "left" || pageEntry === "right" ? (
				<PlaceholderPageButton pagesOmitted={pagesOmitted} />
			) : (
				<NumberedPageButton
					pageNumber={pageEntry}
					highlighted={pageEntry === currentPage}
					onClick={() => onChange(pageEntry)}
				/>
			)}
		</li>
	));
};

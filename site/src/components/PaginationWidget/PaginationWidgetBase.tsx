import { KeyboardArrowLeft as KeyboardArrowLeft, KeyboardArrowRight as KeyboardArrowRight } from "lucide-react";
import { useTheme } from "@emotion/react";
import useMediaQuery from "@mui/material/useMediaQuery";
import type { FC } from "react";
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
};

export const PaginationWidgetBase: FC<PaginationWidgetBaseProps> = ({
	currentPage,
	pageSize,
	totalRecords,
	onPageChange,
	hasPreviousPage,
	hasNextPage,
}) => {
	const theme = useTheme();
	const isMobile = useMediaQuery(theme.breakpoints.down("md"));
	const totalPages = Math.ceil(totalRecords / pageSize);

	if (totalPages < 2) {
		return null;
	}

	const currentPageOffset = (currentPage - 1) * pageSize;
	const isPrevDisabled = !(hasPreviousPage ?? currentPage > 1);
	const isNextDisabled = !(
		hasNextPage ?? pageSize + currentPageOffset < totalRecords
	);

	return (
		<div
			css={{
				justifyContent: "center",
				alignItems: "center",
				display: "flex",
				flexDirection: "row",
				padding: "0 20px",
				columnGap: "6px",
			}}
		>
			<PaginationNavButton
				disabledMessage="You are already on the first page"
				disabled={isPrevDisabled}
				aria-label="Previous page"
				onClick={() => {
					if (!isPrevDisabled) {
						onPageChange(currentPage - 1);
					}
				}}
			>
				<KeyboardArrowLeft />
			</PaginationNavButton>

			{isMobile ? (
				<NumberedPageButton
					highlighted
					pageNumber={currentPage}
					totalPages={totalPages}
				/>
			) : (
				<PaginationRow
					currentPage={currentPage}
					totalPages={totalPages}
					onChange={onPageChange}
				/>
			)}

			<PaginationNavButton
				disabledMessage="You are already on the last page"
				disabled={isNextDisabled}
				aria-label="Next page"
				onClick={() => {
					if (!isNextDisabled) {
						onPageChange(currentPage + 1);
					}
				}}
			>
				<KeyboardArrowRight />
			</PaginationNavButton>
		</div>
	);
};

type PaginationRowProps = {
	currentPage: number;
	totalPages: number;
	onChange: (newPage: number) => void;
};

const PaginationRow: FC<PaginationRowProps> = ({
	currentPage,
	totalPages,
	onChange,
}) => {
	const pageInfo = buildPagedList(totalPages, currentPage);
	const pagesOmitted = totalPages - pageInfo.length - 1;

	return (
		<>
			{pageInfo.map((pageEntry) => {
				if (pageEntry === "left" || pageEntry === "right") {
					return (
						<PlaceholderPageButton
							key={pageEntry}
							pagesOmitted={pagesOmitted}
						/>
					);
				}

				return (
					<NumberedPageButton
						key={pageEntry}
						pageNumber={pageEntry}
						totalPages={totalPages}
						highlighted={pageEntry === currentPage}
						onClick={() => onChange(pageEntry)}
					/>
				);
			})}
		</>
	);
};

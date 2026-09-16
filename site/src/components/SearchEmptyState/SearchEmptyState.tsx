import type { FC, SVGProps } from "react";
import { Button } from "#/components/Button/Button";
import { TableCell, TableRow } from "#/components/Table/Table";

const SearchGlyphIcon: FC<SVGProps<SVGSVGElement>> = (props) => {
	return (
		<svg
			viewBox="0 0 36 36"
			fill="none"
			xmlns="http://www.w3.org/2000/svg"
			aria-hidden="true"
			{...props}
		>
			<path
				d="M15.7498 23.4C20.9689 23.4 25.1998 19.1691 25.1998 13.95C25.1998 8.73091 20.9689 4.5 15.7498 4.5C10.5307 4.5 6.2998 8.73091 6.2998 13.95C6.2998 19.1691 10.5307 23.4 15.7498 23.4Z"
				stroke="currentColor"
				strokeWidth={1.99528}
				strokeLinecap="round"
				strokeLinejoin="round"
			/>
			<path
				d="M29.5277 31.4998L30.2948 30.8619L28.3472 28.5131L27.5801 29.151L26.813 29.7889L28.7606 32.1376L29.5277 31.4998Z"
				fill="currentColor"
			/>
			<path
				d="M26.2815 27.5852L27.0485 26.9472L21.8549 20.6839L21.0879 21.3219L20.3209 21.9599L25.5146 28.2232L26.2815 27.5852Z"
				fill="currentColor"
			/>
		</svg>
	);
};

interface SearchEmptyStateProps {
	/** Contextual heading, e.g. "No users match your search". */
	message: string;
	/** Optional supporting copy shown below the heading. */
	description?: string;
	/** When provided, renders a "Reset filters" button that clears the search. */
	onClearFilters?: () => void;
}

/**
 * Empty state shown when a search or filter returns no matching rows. It keeps
 * the messaging distinct from a truly empty list so admins know results exist
 * but are hidden by the current filter.
 */
export const SearchEmptyState: FC<SearchEmptyStateProps> = ({
	message,
	description = "Broaden your search to explore more options.",
	onClearFilters,
}) => {
	return (
		<div className="flex flex-col items-center justify-center gap-4 p-16 text-center">
			<div className="flex size-12 items-center justify-center rounded-md bg-surface-sky">
				<SearchGlyphIcon className="size-9 text-highlight-sky" />
			</div>
			<div className="flex flex-col gap-2">
				<h4 className="m-0 font-semibold text-content-primary text-sm">
					{message}
				</h4>
				{description && (
					<p className="m-0 max-w-[420px] text-content-secondary text-sm">
						{description}
					</p>
				)}
			</div>
			{onClearFilters && (
				<Button variant="outline" size="sm" onClick={onClearFilters}>
					Reset filters
				</Button>
			)}
		</div>
	);
};

/**
 * Renders {@link SearchEmptyState} inside a full-width table row so pages can
 * keep their table header visible while showing the empty state.
 */
export const TableSearchEmpty: FC<SearchEmptyStateProps> = (props) => {
	return (
		<TableRow>
			<TableCell colSpan={999}>
				<SearchEmptyState {...props} />
			</TableCell>
		</TableRow>
	);
};

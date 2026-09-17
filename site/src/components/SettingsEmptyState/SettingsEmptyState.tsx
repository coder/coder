import type { FC, ReactNode, SVGProps } from "react";
import { TableCell, TableRow } from "#/components/Table/Table";

/**
 * Glyph shown in the badge of the admin settings empty states.
 */
const SettingsEmptyIcon: FC<SVGProps<SVGSVGElement>> = (props) => {
	return (
		<svg
			viewBox="0 0 21 21"
			fill="none"
			xmlns="http://www.w3.org/2000/svg"
			aria-hidden="true"
			{...props}
		>
			<path
				d="M10.2012 1.20154V19.2015"
				stroke="currentColor"
				strokeWidth={2.403}
				strokeLinecap="square"
				strokeLinejoin="round"
			/>
			<path
				d="M19.2012 10.2015H1.20117"
				stroke="currentColor"
				strokeWidth={2.403}
				strokeLinecap="square"
				strokeLinejoin="round"
			/>
		</svg>
	);
};

interface SettingsEmptyStateProps {
	/** Contextual heading, e.g. "No models configured". */
	message: string;
	/** Optional supporting copy shown below the heading. */
	description?: string;
	/** Optional action, e.g. an "Add" button, rendered below the copy. */
	cta?: ReactNode;
}

/**
 * Empty state shown on admin settings pages when a resource has not been
 * configured yet. It shares the icon badge, typography, and spacing used by the
 * search and IdP sync empty states so the admin experience stays consistent.
 */
export const SettingsEmptyState: FC<SettingsEmptyStateProps> = ({
	message,
	description,
	cta,
}) => {
	return (
		<div className="flex flex-col items-center justify-center gap-4 p-16 text-center">
			<div className="flex size-12 items-center justify-center rounded-md bg-surface-sky">
				<SettingsEmptyIcon className="size-icon-sm text-highlight-sky" />
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
			{cta}
		</div>
	);
};

/**
 * Renders {@link SettingsEmptyState} inside a full-width table row so pages can
 * keep their table header visible while showing the empty state.
 */
export const TableSettingsEmpty: FC<SettingsEmptyStateProps> = (props) => {
	return (
		<TableRow>
			<TableCell colSpan={999}>
				<SettingsEmptyState {...props} />
			</TableCell>
		</TableRow>
	);
};

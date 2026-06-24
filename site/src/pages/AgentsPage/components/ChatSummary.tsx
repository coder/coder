import type { FC, ReactNode } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatCostMicros } from "#/utils/currency";
import { formatDate } from "#/utils/time";

const EMPTY_VALUE = "-";

interface ChatSummaryProps {
	/**
	 * Short summary of the chat. Renders a muted empty state when null or blank
	 * so the component can be wired to a real summary source later without UI
	 * changes.
	 */
	summary: string | null;
	createdAt: string;
	updatedAt: string;
	/** Cumulative chat cost in microdollars (1 USD = 1,000,000 micros). */
	costMicros?: number | null;
	isCostLoading?: boolean;
}

/**
 * ChatSummary is a presentational, reusable summary of a chat: a short summary
 * blurb plus its created/updated dates and cumulative cost. It performs no data
 * fetching so it can be dropped in anywhere and exercised in Storybook.
 */
export const ChatSummary: FC<ChatSummaryProps> = ({
	summary,
	createdAt,
	updatedAt,
	costMicros,
	isCostLoading,
}) => {
	const trimmedSummary = summary?.trim();

	return (
		<div className="flex flex-col gap-4">
			<h3 className="m-0 text-lg font-semibold text-content-primary">
				Summary
			</h3>

			{trimmedSummary ? (
				<p className="m-0 text-sm leading-relaxed text-content-primary">
					{trimmedSummary}
				</p>
			) : (
				<p className="m-0 text-sm italic text-content-secondary">
					No summary yet.
				</p>
			)}

			<dl className="m-0 flex flex-col gap-2">
				<ChatSummaryRow label="Created:">
					{formatDate(new Date(createdAt))}
				</ChatSummaryRow>
				<ChatSummaryRow label="Updated:">
					{formatDate(new Date(updatedAt))}
				</ChatSummaryRow>
				<ChatSummaryRow label="Cost:">
					{isCostLoading ? (
						<Skeleton aria-label="Loading cost" className="h-4 w-16" />
					) : costMicros != null ? (
						formatCostMicros(costMicros)
					) : (
						EMPTY_VALUE
					)}
				</ChatSummaryRow>
			</dl>
		</div>
	);
};

interface ChatSummaryRowProps {
	label: string;
	children: ReactNode;
}

const ChatSummaryRow: FC<ChatSummaryRowProps> = ({ label, children }) => (
	<div className="flex items-center justify-between gap-4 text-sm">
		<dt className="text-content-secondary">{label}</dt>
		<dd className="m-0 font-medium text-content-primary">{children}</dd>
	</div>
);

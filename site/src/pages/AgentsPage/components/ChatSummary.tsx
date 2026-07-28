import type { FC, ReactNode } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

const EMPTY_VALUE = "-";

interface ChatSummaryProps {
	summary: string | null;
	createdAt: string;
	updatedAt: string;
	/** Cumulative chat cost in microdollars (1 USD = 1,000,000). */
	costMicros?: number | null;
	isCostLoading?: boolean;
	costError?: boolean;
	/** Assistant messages with usage but no model pricing; when > 0 the cost is partial and a note is shown. */
	unpricedMessagesHavingUsageCount?: number;
	/** Subagent summaries are the agent's final report, persisted when it completes, so the empty state reads as pending rather than absent. */
	isSubagent?: boolean;
}

export const ChatSummary: FC<ChatSummaryProps> = ({
	summary,
	createdAt,
	updatedAt,
	costMicros,
	isCostLoading,
	costError,
	unpricedMessagesHavingUsageCount,
	isSubagent,
}) => {
	const trimmedSummary = summary?.trim();
	const hasUnpricedMessages =
		!isCostLoading &&
		!costError &&
		costMicros != null &&
		unpricedMessagesHavingUsageCount != null &&
		unpricedMessagesHavingUsageCount > 0;

	return (
		<div className="flex flex-col gap-4">
			{trimmedSummary ? (
				<p className="m-0 font-sans text-pretty text-sm font-normal leading-6 text-content-primary">
					{trimmedSummary}
				</p>
			) : (
				<p className="m-0 font-sans text-sm font-normal leading-6 text-content-secondary">
					{isSubagent ? "Summary pending agent completion." : "No summary yet."}
				</p>
			)}

			<dl className="m-0 flex flex-col gap-1.5">
				<ChatSummaryRow label="Created:">
					{formatDateTime(createdAt, DATE_FORMAT.MEDIUM_DATE)}
				</ChatSummaryRow>
				<ChatSummaryRow label="Updated:">
					{formatDateTime(updatedAt, DATE_FORMAT.MEDIUM_DATE)}
				</ChatSummaryRow>
				<ChatSummaryRow label="Cost:">
					{isCostLoading ? (
						<Skeleton aria-label="Loading cost" className="my-1 h-4 w-16" />
					) : costError ? (
						<span className="text-content-secondary">Unavailable</span>
					) : costMicros != null ? (
						formatCostMicros(costMicros)
					) : (
						EMPTY_VALUE
					)}
				</ChatSummaryRow>
			</dl>

			{hasUnpricedMessages && (
				<p className="m-0 text-xs italic text-content-secondary">
					Excludes {unpricedMessagesHavingUsageCount} message
					{unpricedMessagesHavingUsageCount === 1 ? "" : "s"} with usage but
					without model pricing.
				</p>
			)}
		</div>
	);
};

interface ChatSummaryRowProps {
	label: string;
	children: ReactNode;
}

const ChatSummaryRow: FC<ChatSummaryRowProps> = ({ label, children }) => (
	<div className="grid grid-cols-[65px_minmax(0,1fr)] gap-x-2 text-sm leading-6">
		<dt className="text-content-secondary">{label}</dt>
		<dd className="m-0 font-sans text-sm font-normal leading-6 text-content-primary">
			{children}
		</dd>
	</div>
);

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
	/** Assistant messages with no model pricing; when > 0 the cost is partial and a note is shown. */
	unpricedMessageCount?: number;
}

export const ChatSummary: FC<ChatSummaryProps> = ({
	summary,
	createdAt,
	updatedAt,
	costMicros,
	isCostLoading,
	costError,
	unpricedMessageCount,
}) => {
	const trimmedSummary = summary?.trim();
	const hasUnpricedMessages =
		!isCostLoading &&
		!costError &&
		costMicros != null &&
		unpricedMessageCount != null &&
		unpricedMessageCount > 0;

	return (
		<div className="flex flex-col gap-4">
			{trimmedSummary ? (
				<p className="m-0 font-sans text-pretty text-sm font-normal leading-6 text-content-primary">
					{trimmedSummary}
				</p>
			) : (
				<p className="m-0 font-sans text-sm font-normal leading-6 text-content-secondary">
					No summary yet.
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
						<Skeleton aria-label="Loading cost" className="h-4 w-16" />
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
					Excludes {unpricedMessageCount} message
					{unpricedMessageCount === 1 ? "" : "s"} without model pricing.
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

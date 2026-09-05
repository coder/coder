import { ExternalLinkIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

const EMPTY_VALUE = "-";

/** A live port-forward link derived from the workspace's listening ports. */
export interface ChatSummaryPreviewLink {
	label: string;
	port: number;
	url: string;
}

export interface ChatSummaryProps {
	summary: string | null;
	createdAt: string;
	updatedAt: string;
	/** Cost of the whole chat tree in microdollars (1 USD = 1,000,000). */
	costMicros?: number | null;
	isCostLoading?: boolean;
	costError?: boolean;
	/** Requests with usage the gateway could not price, so the reported cost is partial. */
	unpricedRequestCount?: number;
	showCost: boolean;
	/** Subagent summaries are the agent's final report, persisted when it completes, so the empty state reads as pending rather than absent. */
	isSubagent?: boolean;
	previews?: readonly ChatSummaryPreviewLink[];
}

export const ChatSummary: FC<ChatSummaryProps> = ({
	summary,
	createdAt,
	updatedAt,
	costMicros,
	isCostLoading,
	costError,
	unpricedRequestCount,
	showCost,
	isSubagent,
	previews,
}) => {
	const trimmedSummary = summary?.trim();
	const hasCost =
		showCost && !isCostLoading && !costError && costMicros != null;
	const hasUnpricedRequests =
		hasCost && unpricedRequestCount != null && unpricedRequestCount > 0;

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
				{previews && previews.length > 0 && (
					<ChatSummaryRow label="Preview:">
						<div className="flex flex-col">
							{previews.map((preview) => (
								<a
									key={preview.port}
									href={preview.url}
									target="_blank"
									rel="noreferrer"
									className="inline-flex items-center gap-1 text-content-link no-underline hover:underline"
								>
									{preview.label} ({preview.port})
									<ExternalLinkIcon aria-hidden className="size-3 shrink-0" />
								</a>
							))}
						</div>
					</ChatSummaryRow>
				)}
				{showCost && (
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
				)}
			</dl>

			{isSubagent && hasCost && (
				<p className="m-0 text-xs italic text-content-secondary">
					Cost covers this agent's whole chat, including the chat that started
					it and any other subagents.
				</p>
			)}

			{hasUnpricedRequests && (
				<p className="m-0 text-xs italic text-content-secondary">
					Excludes unpriced usage from {unpricedRequestCount} request
					{unpricedRequestCount === 1 ? "" : "s"}.
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

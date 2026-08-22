import type { FC, ReactNode } from "react";
import { InlineMarkdown } from "#/components/Markdown/InlineMarkdown";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

const EMPTY_VALUE = "-";

/** Compact list spacing that keeps markers inside the narrow summary column. */
const LIST_CLASSES = "my-2 flex flex-col gap-1 pl-5";

interface ChatSummaryProps {
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
}) => {
	const trimmedSummary = summary?.trim();
	const hasCost =
		showCost && !isCostLoading && !costError && costMicros != null;
	const hasUnpricedRequests =
		hasCost && unpricedRequestCount != null && unpricedRequestCount > 0;

	return (
		<div className="flex flex-col gap-4">
			{trimmedSummary ? (
				<ChatSummaryBody summary={trimmedSummary} />
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

interface ChatSummaryBodyProps {
	summary: string;
}

/**
 * Renders the stored summary markdown: a headline paragraph plus an optional
 * bullet list.
 *
 * The height is deliberately unbounded. Generated summaries are capped at 600
 * runes server-side, and the panel around this is already a scroll container,
 * so clamping would only put a second, worse overflow mechanism in front of
 * content the reader can already reach by scrolling.
 */
const ChatSummaryBody: FC<ChatSummaryBodyProps> = ({ summary }) => (
	<div
		// Identifiers are preserved verbatim and can exceed the panel width with
		// no natural break opportunity. Breaking anywhere keeps them inside the
		// column instead of widening the box past the panel.
		className="w-full break-words font-sans text-sm font-normal leading-6 text-content-primary [overflow-wrap:anywhere]"
	>
		<InlineMarkdown
			// `ol` is allowed alongside `ul` so a legacy prose summary that
			// happens to start with "1. " still nests its items in a list;
			// disallowing it emits `li` elements with no list parent.
			allowedElements={["ul", "ol", "li"]}
			components={{
				// InlineMarkdown renders `p` as a bare fragment, which would run
				// the headline straight into the bullet list.
				p: ({ children }) => <p className="m-0 text-pretty">{children}</p>,
				ul: ({ children }) => (
					<ul className={`${LIST_CLASSES} list-disc`}>{children}</ul>
				),
				ol: ({ children }) => (
					<ol className={`${LIST_CLASSES} list-decimal`}>{children}</ol>
				),
				li: ({ children }) => <li className="m-0 text-pretty">{children}</li>,
				// Render link text without an anchor. A summary describes the chat
				// rather than linking out of it, and any URL here is model-authored,
				// so it is not a navigation target worth mounting.
				a: ({ children }) => <>{children}</>,
			}}
		>
			{summary}
		</InlineMarkdown>
	</div>
);

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

import { MessageSquareDashedIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { InlineMarkdown } from "#/components/Markdown/InlineMarkdown";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { Shimmer } from "./ChatElements";

const EMPTY_VALUE = "-";
const EMPTY_SUMMARY_TITLE = "Not enough details to summarize.";
const EMPTY_SUMMARY_DESCRIPTION =
	"A recap of your chat will appear here after a few more messages.";

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
	/** True while a root-chat summary is expected to land after a finished turn. */
	isGenerating?: boolean;
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
	isGenerating,
}) => {
	const trimmedSummary = summary?.trim();
	const hasCost =
		showCost && !isCostLoading && !costError && costMicros != null;
	const hasUnpricedRequests =
		hasCost && unpricedRequestCount != null && unpricedRequestCount > 0;

	return (
		<div className="flex min-h-0 flex-col gap-4 p-4">
			{trimmedSummary ? (
				<ChatSummaryBody summary={trimmedSummary} />
			) : isSubagent ? (
				<p className="m-0 font-sans text-sm font-normal leading-6 text-content-secondary">
					Summary pending agent completion.
				</p>
			) : (
				<ChatSummaryEmpty
					title={
						isGenerating ? (
							<Shimmer as="span" className="text-sm font-medium">
								Generating summary
							</Shimmer>
						) : (
							EMPTY_SUMMARY_TITLE
						)
					}
					description={isGenerating ? undefined : EMPTY_SUMMARY_DESCRIPTION}
				/>
			)}

			<dl className="m-0 flex shrink-0 flex-col gap-1.5">
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

interface ChatSummaryEmptyProps {
	title: ReactNode;
	description?: string;
}

const ChatSummaryEmpty: FC<ChatSummaryEmptyProps> = ({
	title,
	description,
}) => (
	<div className="flex flex-col items-center px-4 py-8 text-center">
		<div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-solid border-border-default bg-surface-secondary">
			<MessageSquareDashedIcon
				aria-hidden
				className="size-5 text-content-secondary"
			/>
		</div>
		<p className="m-0 text-sm font-medium text-content-primary">{title}</p>
		<p className="mt-1 min-h-8 max-w-56 text-xs text-content-secondary">
			{description}
		</p>
	</div>
);

interface ChatSummaryBodyProps {
	summary: string;
}

/**
 * Height is deliberately unbounded: summaries are capped server-side and the
 * surrounding panel already scrolls, so clamping would only add a second,
 * worse overflow mechanism.
 */
const ChatSummaryBody: FC<ChatSummaryBodyProps> = ({ summary }) => (
	<div
		// Verbatim identifiers can exceed the panel width with no natural
		// break opportunity, so break anywhere.
		className="w-full break-words font-sans text-sm font-normal leading-6 text-content-primary [overflow-wrap:anywhere]"
	>
		<InlineMarkdown
			// `ol` keeps a legacy prose summary starting with "1. " in a list
			// parent instead of emitting orphan `li` elements.
			allowedElements={["ul", "ol", "li"]}
			components={{
				// InlineMarkdown renders `p` as a bare fragment, which would
				// run the headline straight into the bullet list.
				p: ({ children }) => <p className="m-0 text-pretty">{children}</p>,
				ul: ({ children }) => (
					<ul className="my-2 flex list-disc flex-col gap-1 pl-5">
						{children}
					</ul>
				),
				ol: ({ children }) => (
					<ol className="my-2 flex list-decimal flex-col gap-1 pl-5">
						{children}
					</ol>
				),
				li: ({ children }) => <li className="m-0 text-pretty">{children}</li>,
				// A summary describes the chat rather than linking out of it, so
				// model-authored URLs render as plain text.
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

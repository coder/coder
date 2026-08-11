import {
	type FC,
	type ReactNode,
	useCallback,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
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
 * Renders the stored summary markdown (a headline paragraph plus an optional
 * bullet list) inside a bounded box, revealing a toggle only when the content
 * actually overflows. `max-height` is used instead of `line-clamp` because
 * `line-clamp` relies on `display: -webkit-box`, which clamps unreliably once
 * the content contains nested block children such as `<ul><li>`.
 */
const ChatSummaryBody: FC<ChatSummaryBodyProps> = ({ summary }) => {
	const contentRef = useRef<HTMLDivElement>(null);
	const [isExpanded, setIsExpanded] = useState(false);
	const [isOverflowing, setIsOverflowing] = useState(false);

	const measure = useCallback(() => {
		const content = contentRef.current;
		if (!content) {
			return;
		}
		// Measure against the collapsed bound, which only applies while
		// collapsed; once expanded the box grows and would always measure as
		// fitting, hiding the "Show less" affordance.
		setIsOverflowing(content.scrollHeight > content.clientHeight);
	}, []);

	useLayoutEffect(() => {
		if (isExpanded) {
			return;
		}
		measure();

		const content = contentRef.current;
		if (!content || typeof ResizeObserver === "undefined") {
			return;
		}
		// The right panel is resizable, and the observer fires immediately on
		// observe(), so this also covers a summary swap that changes the box
		// height. A swap that leaves the height pinned to the clamp cannot
		// change the overflow verdict, so no summary dependency is needed.
		const observer = new ResizeObserver(measure);
		observer.observe(content);
		return () => observer.disconnect();
	}, [measure, isExpanded]);

	return (
		<div className="flex flex-col items-start gap-1">
			<div
				ref={contentRef}
				className={`overflow-hidden font-sans text-sm font-normal leading-6 text-content-primary ${
					isExpanded ? "" : "max-h-48"
				}`}
			>
				<InlineMarkdown
					// `ol` is allowed alongside `ul` so a legacy prose summary that
					// happens to start with "1. " still nests its items in a list;
					// disallowing it emits `li` elements with no list parent.
					allowedElements={["ul", "ol", "li"]}
					components={{
						// InlineMarkdown renders `p` as a bare fragment, which would
						// run the headline straight into the bullet list.
						p: ({ children }) => <p className="m-0 text-pretty">{children}</p>,
						ul: ({ children }) => (
							<ul className={`${LIST_CLASSES} list-disc`}>{children}</ul>
						),
						ol: ({ children }) => (
							<ol className={`${LIST_CLASSES} list-decimal`}>{children}</ol>
						),
						li: ({ children }) => (
							<li className="m-0 text-pretty">{children}</li>
						),
					}}
				>
					{summary}
				</InlineMarkdown>
			</div>

			{(isOverflowing || isExpanded) && (
				<button
					type="button"
					className="cursor-pointer border-0 bg-transparent p-0 font-sans text-xs font-medium text-content-link transition-colors hover:underline"
					onClick={() => setIsExpanded((expanded) => !expanded)}
				>
					{isExpanded ? "Show less" : "Show more"}
				</button>
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

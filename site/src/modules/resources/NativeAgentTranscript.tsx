import { cn } from "cn";
import { useEffect, useRef } from "react";
import type { Components } from "react-markdown";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Table, TableHead } from "#/components/Table/Table";
import type { NativeAgentEntry } from "./NativeAgentSession";

const transcriptMarkdownComponents: Components = {
	// Untrusted transcript images must not trigger requests from the viewer.
	img: ({ alt }) => <span>[Image: {alt || "not displayed"}]</span>,
	pre: ({ children }) => (
		<pre className="whitespace-pre-wrap rounded-md bg-surface-primary p-3 [&_code]:bg-transparent [&_code]:p-0">
			{children}
		</pre>
	),
	code: ({ children }) => (
		<code className="rounded-sm bg-border px-1 py-px font-mono">
			{children}
		</code>
	),
	table: ({ children }) => (
		<Table aria-label="Transcript table">{children}</Table>
	),
	th: ({ children }) => <TableHead>{children}</TableHead>,
};

type NativeAgentTranscriptProps = {
	entries: readonly NativeAgentEntry[];
};

/** Follows streamed text unless the reader scrolls back to inspect history. */
export const NativeAgentTranscript: React.FC<NativeAgentTranscriptProps> = ({
	entries,
}) => {
	const ref = useRef<HTMLDivElement>(null);
	const follow = useRef(true);
	useEffect(() => {
		if (follow.current && ref.current)
			ref.current.scrollTop = ref.current.scrollHeight;
	}, [entries]);
	return (
		<ScrollArea
			viewportRef={ref}
			viewportTabIndex={0}
			viewportAriaLabel="Native agent transcript"
			className="h-64 rounded-md border border-solid border-border bg-surface-secondary"
			viewportClassName="p-3 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-content-link"
			onScrollCapture={() => {
				const element = ref.current;
				if (element)
					follow.current =
						element.scrollHeight - element.scrollTop - element.clientHeight <
						40;
			}}
		>
			<div
				role="log"
				aria-label="Messages"
				aria-live="off"
				className="space-y-3 font-mono"
			>
				{entries.length === 0 && (
					<p className="m-0 text-content-secondary">
						No retained messages in this session.
					</p>
				)}
				{entries.map((entry) => (
					<div
						key={entry.id}
						className={cn(
							"wrap-anywhere",
							entry.kind === "tool" && "whitespace-pre-wrap",
							entry.kind === "thought" && "text-content-secondary",
						)}
					>
						{entry.kind === "tool" ? (
							<>
								<strong>{entry.title || "Tool"}</strong>
								<span className="ml-2 text-content-secondary">
									{entry.status}
								</span>
							</>
						) : (
							<>
								<strong className="block text-content-secondary">
									{entry.kind === "user"
										? "You"
										: entry.kind === "thought"
											? "Thinking"
											: "Agent"}
								</strong>
								<MemoizedMarkdown
									className={cn(
										"text-sm [&>:first-child]:mt-0 [&>:last-child]:mb-0",
										"[&_ul]:list-disc [&_ol]:list-decimal [&_:is(ul,ol)]:pl-6",
										"[&_blockquote]:border-0 [&_blockquote]:border-l-2 [&_blockquote]:border-solid [&_blockquote]:border-border [&_blockquote]:pl-3",
										"[&_:is(h1,h2,h3,h4,h5,h6)]:font-semibold [&_h1]:text-xl [&_h2]:text-lg [&_h3]:text-base",
									)}
									components={transcriptMarkdownComponents}
								>
									{entry.text}
								</MemoizedMarkdown>
							</>
						)}
					</div>
				))}
			</div>
		</ScrollArea>
	);
};

import type { FileDiffMetadata } from "@pierre/diffs";
import { cn } from "cn";
import { type FC, useId } from "react";
import { Response } from "../ChatElements/Response";
import { RenderedMarkdownErrorBoundary } from "./RenderedMarkdownErrorBoundary";
import {
	collectRenderedSegments,
	renderedMarkdownUrlTransform,
} from "./renderedMarkdown";

interface RenderedMarkdownFileProps {
	fileDiff: FileDiffMetadata;
}

/**
 * Markdown preview for one file of a diff: the new side of each hunk, or
 * the old side of a deleted file. Lives in a CodeView annotation slot,
 * whose parent sets the diff's monospace font variables, so typography is
 * reset here.
 */
export const RenderedMarkdownFile: FC<RenderedMarkdownFileProps> = ({
	fileDiff,
}) => {
	const labelIdBase = useId();
	const rendered = collectRenderedSegments(fileDiff);
	const isDeleted = fileDiff.type === "deleted";

	return (
		<section
			aria-label={`Markdown preview of ${fileDiff.name}`}
			className={cn(
				"flex flex-col gap-4 px-2.5 py-3 font-sans text-[13px] leading-relaxed text-content-primary",
				isDeleted && "bg-surface-git-deleted/30",
			)}
		>
			{isDeleted ? (
				<p className="m-0 text-xs text-git-deleted-bright">
					File deleted. Showing the removed content.
				</p>
			) : (
				!rendered.isComplete && (
					<p className="m-0 text-xs text-content-secondary">
						Preview of changed sections in the new version. Deleted lines are
						not shown.
					</p>
				)
			)}
			{/* Keyed by content so a parse failure clears once the file changes. */}
			<RenderedMarkdownErrorBoundary key={fileDiff.cacheKey}>
				{rendered.segments.map((segment, index) => {
					const body = (
						<Response
							className="max-w-3xl"
							urlTransform={renderedMarkdownUrlTransform}
						>
							{segment.text}
						</Response>
					);
					if (rendered.isComplete) {
						return <div key={`${segment.startLine}-${index}`}>{body}</div>;
					}
					const labelId = `${labelIdBase}-${index}`;
					return (
						<div
							key={`${segment.startLine}-${index}`}
							role="group"
							aria-labelledby={labelId}
							className="flex flex-col gap-1"
						>
							<span
								id={labelId}
								className="font-mono text-[11px] text-content-secondary"
							>
								Lines {segment.startLine}-{segment.endLine}
							</span>
							{body}
						</div>
					);
				})}
			</RenderedMarkdownErrorBoundary>
		</section>
	);
};

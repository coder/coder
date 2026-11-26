import { cn } from "cn";
import { memo, useEffectEvent, useLayoutEffect, useRef, useState } from "react";
import type { UrlTransform } from "streamdown";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import { Response, showsResponseText } from "../ChatElements/Response";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import { useSmoothStreamingText } from "./SmoothText";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";

/** Displays reasoning using the reader's expansion and streaming preferences. */
export const ReasoningDisclosure = memo<{
	id: string;
	text: string;
	isStreaming: boolean;
	urlTransform?: UrlTransform;
	thinkingDisplayMode?: ThinkingDisplayMode;
}>(
	({
		id,
		text,
		isStreaming,
		urlTransform,
		thinkingDisplayMode: mode = "auto",
	}) => {
		const [manualToggle, setManualToggle] = useState<boolean | null>(null);

		// Reset manual override on streaming transitions so
		// auto/preview modes collapse when streaming stops.
		const [prevStreaming, setPrevStreaming] = useState(isStreaming);
		if (prevStreaming !== isStreaming) {
			setPrevStreaming(isStreaming);
			if (mode === "auto" || mode === "preview") {
				setManualToggle(null);
			}
		}

		const autoExpanded = (() => {
			switch (mode) {
				case "always_expanded":
					return true;
				case "always_collapsed":
					return false;
				case "auto":
				case "preview":
					return isStreaming;
				default: {
					const _exhaustive: never = mode;
					return _exhaustive;
				}
			}
		})();

		const expanded = manualToggle ?? autoExpanded;

		const isPreviewConstrained =
			mode === "preview" && isStreaming && manualToggle === null;

		const previewScrollRef = useRef<HTMLDivElement>(null);

		const { visibleText } = useSmoothStreamingText({
			fullText: text,
			isStreaming,
			bypassSmoothing: !isStreaming,
			streamKey: id,
		});
		const displayText = isStreaming ? visibleText : text;
		const { title, ariaLabel, body } = getThinkingDisclosureDisplay(
			displayText,
			{
				isStreaming,
			},
		);
		const hasText = body.trim().length > 0;

		// Auto-scroll the preview container to the bottom as new
		// thinking content streams in. useLayoutEffect avoids a
		// visible frame where content has grown but not scrolled.
		const displayTextLength = body.length;
		useLayoutEffect(() => {
			if (
				displayTextLength &&
				isPreviewConstrained &&
				previewScrollRef.current
			) {
				previewScrollRef.current.scrollTop =
					previewScrollRef.current.scrollHeight;
			}
		}, [displayTextLength, isPreviewConstrained]);

		return (
			<div data-transcript-row="">
				<ToolCall.Root
					className="w-full"
					status={isStreaming ? "running" : "completed"}
					hasContent={hasText}
					expanded={expanded}
					onExpandedChange={(open) => setManualToggle(open)}
					ariaLabel={ariaLabel}
				>
					<ToolCall.Header
						iconName="thinking"
						label={title}
						showStatus={false}
					/>
					<ToolCall.Content>
						<div
							ref={previewScrollRef}
							className={cn(
								"mt-1.5",
								isPreviewConstrained && "max-h-24 overflow-y-auto",
							)}
						>
							<Response
								className="text-[11px] text-content-secondary"
								urlTransform={urlTransform}
								streaming={isStreaming}
							>
								{body}
							</Response>
						</div>
					</ToolCall.Content>
				</ToolCall.Root>
			</div>
		);
	},
);

/** Smooths live response text and renders the full text once committed. */
export const ResponseBlock = memo<{
	text: string;
	isStreaming: boolean;
	streamKey: string;
	urlTransform?: UrlTransform;
	/**
	 * Called with the stream key when the block starts and stops showing text.
	 * Smoothing delays streamed text, and Markdown syntax alone renders none.
	 */
	onTextShownChange?: (streamKey: string, shown: boolean) => void;
}>(({ text, isStreaming, streamKey, urlTransform, onTextShownChange }) => {
	const { visibleText } = useSmoothStreamingText({
		fullText: text,
		isStreaming,
		bypassSmoothing: !isStreaming,
		streamKey,
	});
	const reportTextShown = useEffectEvent((shown: boolean) =>
		onTextShownChange?.(streamKey, shown),
	);
	const responseRef = useRef<HTMLDivElement>(null);
	const reportedShown = useRef(false);
	useLayoutEffect(() => {
		const response = responseRef.current;
		const shown = Boolean(
			onTextShownChange && response && showsResponseText(response),
		);
		if (shown !== reportedShown.current) {
			reportedShown.current = shown;
			reportTextShown(shown);
		}
	});
	useLayoutEffect(
		() => () => {
			if (reportedShown.current) {
				reportedShown.current = false;
				reportTextShown(false);
			}
		},
		[],
	);
	return (
		<Response
			ref={responseRef}
			streaming={isStreaming}
			urlTransform={urlTransform}
		>
			{isStreaming ? visibleText : text}
		</Response>
	);
});

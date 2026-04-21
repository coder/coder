import { memo } from "react";
import type { UrlTransform } from "streamdown";
import { Response, Shimmer } from "../ChatElements";
import { useSmoothStreamingText } from "./SmoothText";

interface ReasoningDisclosureProps {
	id: string;
	text: string;
	isStreaming?: boolean;
	urlTransform?: UrlTransform;
}

/**
 * Renders a `thinking` / reasoning block in the chat stream.
 *
 * During streaming, reasoning text is smoothed through the same
 * jitter buffer used by response blocks so it arrives at a steady
 * cadence. For historical messages the text is rendered as-is.
 */
export const ReasoningDisclosure = memo<ReasoningDisclosureProps>(
	({ id, text, isStreaming = false, urlTransform }) => {
		const { visibleText } = useSmoothStreamingText({
			fullText: text,
			isStreaming,
			bypassSmoothing: !isStreaming,
			streamKey: id,
		});
		const displayText = isStreaming ? visibleText : text;
		const hasText = displayText.trim().length > 0;

		if (hasText) {
			return (
				<div className="w-full">
					<Response
						className="text-[11px] text-content-secondary"
						urlTransform={urlTransform}
						streaming={isStreaming}
					>
						{displayText}
					</Response>
				</div>
			);
		}

		return (
			<div className="w-full">
				<div className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary">
					<span className="text-sm">
						{isStreaming ? (
							<Shimmer as="span">Thinking...</Shimmer>
						) : (
							"Thinking"
						)}
					</span>
				</div>
			</div>
		);
	},
);

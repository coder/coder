import { ChevronRightIcon, LightbulbIcon } from "lucide-react";
import { memo, type ReactNode, useId, useState } from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "#/utils/cn";
import { Response, Shimmer } from "../ChatElements";
import { useSmoothStreamingText } from "./SmoothText";

interface ReasoningDisclosureProps {
	id: string;
	text: string;
	isStreaming?: boolean;
	urlTransform?: UrlTransform;
}

const renderHeaderLabel = (
	isStreaming: boolean,
	hasRawText: boolean,
): ReactNode => {
	if (isStreaming && !hasRawText) {
		return <Shimmer as="span">Thinking...</Shimmer>;
	}
	if (isStreaming) {
		return <span>Thinking</span>;
	}
	return <span>Thought</span>;
};

/**
 * Renders a `thinking` / reasoning block in the chat stream.
 *
 * Behavior:
 * - Historical messages (`isStreaming=false`) start collapsed so the
 *   chat stream stays scannable. The user can click the header to
 *   reveal the reasoning text.
 * - Live-streaming messages (`isStreaming=true`) start expanded so the
 *   user can watch reasoning arrive. When the stream completes,
 *   `BlockList` unmounts the live instance (keyPrefix="stream") and
 *   mounts a fresh historical one (keyPrefix=message.id) that
 *   naturally starts collapsed.
 *
 * The initial `isOpen` state is seeded from `isStreaming` at mount
 * time and is not re-synced when the prop changes. Transient
 * reconnect phases (`retrying` / `reconnecting`) flip `isStreaming`
 * to `false` and back to `true` on the same mounted instance, so a
 * prop-driven re-sync effect would collapse the block mid-response
 * and clobber the user's manual toggle. The parent's `keyPrefix`
 * convention in `BlockList` is load-bearing: live streams use the
 * stable `"stream"` prefix (preserving state across reconnects) and
 * switch to `message.id` only when the live instance is replaced by
 * its historical counterpart, which forces an unmount/remount into
 * the collapsed default.
 *
 * A future refactor that keeps the same instance mounted across
 * stream to historical must drive the transition with an explicit
 * terminal-phase signal (e.g. an `onStreamComplete` callback), not
 * the `isStreaming` prop, because `isStreaming=false` alone cannot
 * be distinguished from a transient reconnect by this component.
 *
 * Streaming reasoning text is smoothed through the same jitter buffer
 * used by response blocks so it arrives at a steady cadence.
 */
export const ReasoningDisclosure = memo<ReasoningDisclosureProps>(
	({ id, text, isStreaming = false, urlTransform }) => {
		const [isOpen, setIsOpen] = useState(isStreaming);

		const { visibleText } = useSmoothStreamingText({
			fullText: text,
			isStreaming,
			bypassSmoothing: !isStreaming,
			streamKey: id,
		});
		const displayText = isStreaming ? visibleText : text;
		const hasSmoothedText = displayText.trim().length > 0;
		// Drive the header label from the raw text so it doesn't
		// flicker between "Thinking..." and "Thinking" as the smoothed
		// reveal drips in characters.
		const hasRawText = text.trim().length > 0;

		// useId() guarantees a stable, collision-free id for the
		// aria-controls linkage regardless of whether the caller-supplied
		// `id` is globally unique.
		const bodyId = useId();

		return (
			<div className="w-full rounded-lg border border-solid border-border bg-surface-secondary">
				<button
					type="button"
					aria-expanded={isOpen}
					aria-controls={bodyId}
					onClick={() => setIsOpen((v) => !v)}
					className={cn(
						"flex w-full items-center gap-2 px-3 py-2",
						"bg-transparent border-0 text-inherit cursor-pointer",
						"text-sm text-content-secondary transition-colors",
						"hover:text-content-primary",
					)}
				>
					<LightbulbIcon
						data-testid="reasoning-icon"
						className="size-icon-sm shrink-0"
						aria-hidden="true"
					/>
					<span className="flex-1 text-left">
						{renderHeaderLabel(isStreaming, hasRawText)}
					</span>
					<span
						data-testid="reasoning-chevron"
						className={cn(
							"flex shrink-0 items-center justify-center transition-transform duration-200",
							isOpen ? "rotate-90" : "rotate-0",
						)}
					>
						<ChevronRightIcon className="size-icon-sm" aria-hidden="true" />
					</span>
				</button>
				{isOpen && (
					<div
						id={bodyId}
						className="border-t border-solid border-border px-3 py-2"
					>
						{hasSmoothedText ? (
							<Response
								className="text-[11px] text-content-secondary"
								urlTransform={urlTransform}
								streaming={isStreaming}
							>
								{displayText}
							</Response>
						) : (
							<div className="text-[11px] text-content-secondary">
								{isStreaming ? (
									<Shimmer as="span">Thinking...</Shimmer>
								) : (
									"No reasoning recorded."
								)}
							</div>
						)}
					</div>
				)}
			</div>
		);
	},
);

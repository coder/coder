import {
	BrainIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	LoaderIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import type { DisplayReasoning } from "./useTemplateAgent";

interface ReasoningBlockProps {
	reasoning: DisplayReasoning[];
}

/**
 * Collapsible block that shows AI reasoning/thinking traces.
 * Collapsed by default to keep the chat scannable — users can
 * expand to see the full chain of thought. Styled to match
 * ToolCallCard (borderless row with chevron toggle).
 */
export const ReasoningBlock: FC<ReasoningBlockProps> = ({ reasoning }) => {
	const [isOpen, setIsOpen] = useState(false);
	const isStreaming = reasoning.some((r) => r.isStreaming);
	const combinedText = reasoning.map((r) => r.text).join("\n\n");

	if (combinedText.trim().length === 0 && !isStreaming) {
		return null;
	}

	return (
		<div>
			<button
				type="button"
				onClick={() => setIsOpen((prev) => !prev)}
				aria-expanded={isOpen}
				className={cn(
					"flex w-full items-center gap-2 rounded-md px-1 py-1 text-left",
					"cursor-pointer border-none bg-transparent transition-colors",
					"hover:bg-surface-secondary",
				)}
			>
				{isOpen ? (
					<ChevronDownIcon className="size-3.5 text-content-secondary" />
				) : (
					<ChevronRightIcon className="size-3.5 text-content-secondary" />
				)}
				{isStreaming ? (
					<LoaderIcon className="size-3.5 animate-spin text-content-secondary" />
				) : (
					<BrainIcon className="size-3.5 text-content-secondary" />
				)}
				<span className="text-xs font-medium text-content-primary">
					{isStreaming ? "Thinking…" : "Reasoning"}
				</span>
			</button>

			{isOpen && (
				<div className="ml-3 border-0 border-l-2 border-solid border-border pb-1 pl-3 pt-1">
					<pre className="m-0 whitespace-pre-wrap break-words font-sans text-2xs leading-relaxed text-content-secondary">
						{combinedText || "Thinking…"}
					</pre>
				</div>
			)}
		</div>
	);
};

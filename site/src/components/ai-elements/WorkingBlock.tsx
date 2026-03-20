import { ChevronDownIcon } from "lucide-react";
import { type FC, type ReactNode, memo, useEffect, useState } from "react";
import { cn } from "utils/cn";
import { ConversationItem } from "./conversation";
import { Message, MessageContent } from "./message";
import { Shimmer } from "./shimmer";

interface WorkingBlockProps {
	startedAt: string;
	endedAt?: string | null;
	isActive: boolean;
	defaultExpanded?: boolean;
	/**
	 * Called when the user first expands the block. The callback
	 * should return the React content to display (e.g. fetched
	 * tool call messages). Subsequent toggles reuse the result.
	 */
	onExpand?: () => Promise<ReactNode> | ReactNode;
	/** Pre-loaded content to render immediately without fetching. */
	children?: ReactNode;
}

function formatElapsed(ms: number): string {
	const totalSeconds = Math.floor(ms / 1000);
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;
	if (minutes === 0) {
		return `${seconds}s`;
	}
	return `${minutes}m ${seconds}s`;
}

/**
 * WorkingBlock renders a clickable "Working... 4m 36s" shimmer
 * indicator (active) or "Worked for 4m 36s" label (complete)
 * with a chevron disclosure that expands to reveal tool calls.
 */
export const WorkingBlock: FC<WorkingBlockProps> = memo(
	function WorkingBlock({ startedAt, endedAt, isActive, defaultExpanded, onExpand, children }) {
		const [expanded, setExpanded] = useState(defaultExpanded ?? false);
		const [loadedContent, setLoadedContent] = useState<ReactNode>(null);
		const [loading, setLoading] = useState(false);
		const [elapsed, setElapsed] = useState(() => {
			const start = new Date(startedAt).getTime();
			const end = endedAt ? new Date(endedAt).getTime() : Date.now();
			return end - start;
		});

		useEffect(() => {
			if (!isActive) return;
			const start = new Date(startedAt).getTime();
			const interval = setInterval(() => {
				setElapsed(Date.now() - start);
			}, 1000);
			return () => clearInterval(interval);
		}, [isActive, startedAt]);

		const label = isActive
			? `Working... ${formatElapsed(elapsed)}`
			: `Worked for ${formatElapsed(elapsed)}`;

		const handleToggle = async () => {
			const next = !expanded;
			setExpanded(next);
			if (next && !loadedContent && !children && onExpand) {
				setLoading(true);
				try {
					const content = await onExpand();
					setLoadedContent(content);
				} finally {
					setLoading(false);
				}
			}
		};

		const expandedContent = children ?? loadedContent;

		return (
			<ConversationItem role="assistant">
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div>
							<button
								type="button"
								aria-expanded={expanded}
								onClick={handleToggle}
								className={cn(
									"border-0 bg-transparent p-0 m-0 font-[inherit] text-[inherit] text-left",
									"flex w-full items-center gap-2 cursor-pointer",
								)}
							>
								{isActive ? (
									<Shimmer
										as="span"
										className="text-[13px] leading-relaxed"
									>
										{label}
									</Shimmer>
								) : (
									<span className="text-[13px] leading-relaxed text-content-secondary">
										{label}
									</span>
								)}
								<ChevronDownIcon
									className={cn(
										"h-3 w-3 shrink-0 text-content-secondary transition-transform",
										expanded ? "rotate-0" : "-rotate-90",
									)}
								/>
							</button>
							{expanded && loading && (
								<div className="mt-3 text-xs text-content-secondary">
									Loading...
								</div>
							)}
							{expanded && expandedContent && (
								<div className="mt-3 space-y-3">
									{expandedContent}
								</div>
							)}
						</div>
					</MessageContent>
				</Message>
			</ConversationItem>
		);
	},
);

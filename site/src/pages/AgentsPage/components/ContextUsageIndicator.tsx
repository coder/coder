import { cn } from "cn";
import { type FC, useEffect, useRef, useState } from "react";
import type { ChatContext } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { contextUsageLabel } from "../utils/contextUsage";
import { SvgRingProgress } from "./SvgRingProgress";

export type AgentContextUsage = {
	readonly usedTokens?: number;
	readonly estimated?: boolean;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	// Percentage (0-100) at which the context will be compacted.
	readonly compressionThreshold?: number;
	// Pinned workspace-context state: the resources the chat is built from and
	// whether they have drifted from the agent's latest snapshot.
	readonly context?: ChatContext;
};

export const ContextUsageIndicator: FC<{
	usage: AgentContextUsage | null;
	onOpenDetails: (opener: HTMLButtonElement | null) => void;
}> = ({ usage, onOpenDetails }) => {
	const [open, setOpen] = useState(false);
	const triggerRef = useRef<HTMLButtonElement>(null);
	const contentRef = useRef<HTMLDivElement>(null);
	const pointerInside = useRef(false);
	const suppressFocusOpen = useRef(false);
	const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const label = contextUsageLabel(usage);
	const used = usage?.usedTokens;
	const limit = usage?.contextLimitTokens;
	const percent =
		used !== undefined &&
		Number.isFinite(used) &&
		used >= 0 &&
		limit !== undefined &&
		Number.isFinite(limit) &&
		limit > 0
			? (used / limit) * 100
			: 0;

	const cancelClose = () => {
		if (closeTimer.current !== null) clearTimeout(closeTimer.current);
		closeTimer.current = null;
	};
	useEffect(
		() => () => {
			if (closeTimer.current !== null) clearTimeout(closeTimer.current);
		},
		[],
	);
	const scheduleClose = () => {
		cancelClose();
		closeTimer.current = setTimeout(() => {
			const focused = document.activeElement;
			if (
				!pointerInside.current &&
				focused !== triggerRef.current &&
				!contentRef.current?.contains(focused)
			)
				setOpen(false);
		}, 150);
	};
	const enter = () => {
		pointerInside.current = true;
		cancelClose();
		if (!suppressFocusOpen.current) setOpen(true);
	};
	const leave = () => {
		pointerInside.current = false;
		suppressFocusOpen.current = false;
		scheduleClose();
	};
	return (
		<Popover open={open} onOpenChange={setOpen} modal={false}>
			<PopoverTrigger asChild>
				<button
					type="button"
					ref={triggerRef}
					aria-label={`Context usage: ${label}`}
					className={cn(
						"flex size-8 shrink-0 items-center justify-center rounded-full border-0 bg-transparent p-0 cursor-pointer focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link forced-colors:text-[ButtonText]",
						percent >= 95
							? "text-content-destructive"
							: percent >= 85
								? "text-content-warning"
								: "text-content-secondary",
					)}
					onPointerEnter={(event) => {
						if (event.pointerType !== "touch") enter();
					}}
					onPointerLeave={leave}
					onFocus={() => {
						cancelClose();
						if (!suppressFocusOpen.current) setOpen(true);
					}}
					onBlur={scheduleClose}
					onClick={(event) => {
						event.preventDefault();
						suppressFocusOpen.current = false;
						setOpen(true);
					}}
				>
					<SvgRingProgress
						size={21.5}
						strokeWidth={2.25}
						percent={percent}
						trackClassName="stroke-current opacity-30 forced-colors:opacity-100 forced-colors:stroke-[GrayText]"
						progressClassName="stroke-current motion-reduce:transition-none forced-colors:stroke-[ButtonText]"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				disablePortal
				ref={contentRef}
				aria-label="Context usage"
				side="top"
				align="end"
				className="flex w-auto max-w-[calc(100vw-2rem)] items-center gap-3 p-3 text-xs motion-reduce:animate-none forced-colors:text-[CanvasText]"
				onOpenAutoFocus={(event) => event.preventDefault()}
				onCloseAutoFocus={(event) => event.preventDefault()}
				onPointerEnter={enter}
				onPointerLeave={leave}
				onKeyDownCapture={(event) => {
					// This non-modal popup participates in the surrounding tab order.
					if (event.key === "Tab") event.stopPropagation();
				}}
				onFocusCapture={cancelClose}
				onBlurCapture={scheduleClose}
				onFocusOutside={(event) => {
					if (pointerInside.current || event.target === triggerRef.current)
						event.preventDefault();
				}}
				onEscapeKeyDown={() => {
					suppressFocusOpen.current = true;
					cancelClose();
					if (contentRef.current?.contains(document.activeElement))
						triggerRef.current?.focus();
				}}
			>
				<span className="min-w-0">{label}</span>
				<Button
					variant="subtle"
					size="sm"
					className="min-h-6 shrink-0 px-1 text-content-link underline forced-colors:text-[LinkText] forced-colors:focus-visible:outline forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]"
					onClick={() => {
						cancelClose();
						suppressFocusOpen.current = true;
						setOpen(false);
						onOpenDetails(triggerRef.current);
					}}
				>
					Details
				</Button>
			</PopoverContent>
		</Popover>
	);
};

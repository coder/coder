import { FileIcon, ZapIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { ChatMessagePart } from "#/api/typesGenerated";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { isMobileViewport } from "#/utils/mobile";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	// Percentage (0–100) at which the context will be compacted.
	readonly compressionThreshold?: number;
	// Last injected context parts (AGENTS.md files and skills).
	readonly lastInjectedContext?: readonly ChatMessagePart[];
}

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

const formatTokenCount = (value: number | undefined): string =>
	hasFiniteTokenValue(value) ? value.toLocaleString() : "--";

const formatTokenCountCompact = (value: number | undefined): string => {
	if (!hasFiniteTokenValue(value)) {
		return "--";
	}
	if (value >= 1_000_000) {
		const m = value / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1).replace(/\.0$/, "")}M`;
	}
	if (value >= 1_000) {
		const k = value / 1_000;
		return `${Number.isInteger(k) ? k : k.toFixed(1).replace(/\.0$/, "")}K`;
	}
	return String(value);
};

const getIndicatorToneClassName = (percentUsed: number | null): string => {
	if (percentUsed === null) {
		return "text-content-secondary/60";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= 85) {
		return "text-content-warning";
	}
	return "text-content-secondary/60";
};

/** Extract the trailing filename from an absolute path. */
const basename = (path: string): string => {
	const slash = path.lastIndexOf("/");
	return slash >= 0 ? path.substring(slash + 1) : path;
};

const RING_SIZE = 18;
const RING_STROKE = 2.5;
const RING_RADIUS = (RING_SIZE - RING_STROKE) / 2;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

// Delay before the popover closes after the mouse leaves, giving
// the user time to move into the popover content.
const HOVER_CLOSE_DELAY_MS = 150;

export const ContextUsageIndicator: FC<{ usage: AgentContextUsage | null }> = ({
	usage,
}) => {
	const [open, setOpen] = useState(false);
	const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const cancelClose = () => {
		if (closeTimerRef.current) {
			clearTimeout(closeTimerRef.current);
			closeTimerRef.current = null;
		}
	};

	const scheduleClose = () => {
		cancelClose();
		closeTimerRef.current = setTimeout(() => {
			setOpen(false);
			closeTimerRef.current = null;
		}, HOVER_CLOSE_DELAY_MS);
	};

	const handleMouseEnter = () => {
		cancelClose();
		setOpen(true);
	};

	const usedTokens = hasFiniteTokenValue(usage?.usedTokens)
		? usage.usedTokens
		: undefined;
	const contextLimitTokens = hasFiniteTokenValue(usage?.contextLimitTokens)
		? usage.contextLimitTokens
		: undefined;
	const percentUsed =
		usedTokens !== undefined &&
		contextLimitTokens !== undefined &&
		contextLimitTokens > 0
			? (usedTokens / contextLimitTokens) * 100
			: null;
	const hasPercent = percentUsed !== null;
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 100;
	const dashOffset =
		RING_CIRCUMFERENCE - (clampedPercent / 100) * RING_CIRCUMFERENCE;
	const toneClassName = getIndicatorToneClassName(percentUsed);
	const ariaLabel = hasPercent
		? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.`
		: "Context usage";

	// Extract context files and skills from lastInjectedContext.
	const contextFiles =
		usage?.lastInjectedContext?.filter((p) => p.type === "context-file") ?? [];
	const skills =
		usage?.lastInjectedContext?.filter((p) => p.type === "skill") ?? [];
	const hasInjectedContext = contextFiles.length > 0 || skills.length > 0;

	const panelContent = (
		<div className="text-xs text-content-primary">
			{hasPercent
				? `${percentLabel} – ${formatTokenCountCompact(usedTokens)} / ${formatTokenCountCompact(contextLimitTokens)} context used`
				: "Context usage unavailable"}
			{hasPercent &&
				usage?.compressionThreshold !== undefined &&
				usage.compressionThreshold > 0 && (
					<div className="mt-1 text-content-secondary">
						{`Compacts at ${usage.compressionThreshold}%`}
					</div>
				)}
			{hasInjectedContext && (
				<div
					className={cn(
						"flex flex-col gap-2 text-content-secondary",
						hasPercent && "mt-2",
					)}
				>
					{contextFiles.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								Context files
							</span>
							{contextFiles.map((part) => {
								if (part.type !== "context-file") return null;
								return (
									<div
										key={part.context_file_path}
										className="flex items-center gap-1.5"
									>
										<FileIcon className="size-3 shrink-0" />
										<span className="truncate" title={part.context_file_path}>
											{basename(part.context_file_path)}
										</span>
										{part.context_file_truncated && (
											<span className="shrink-0 text-content-warning">
												(truncated)
											</span>
										)}
									</div>
								);
							})}
						</div>
					)}
					{skills.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">Skills</span>
							<TooltipProvider delayDuration={300}>
								{skills.map((part) => {
									if (part.type !== "skill") return null;
									const row = (
										<div className="flex items-center gap-1.5 rounded px-0.5 py-px transition-colors hover:bg-surface-tertiary">
											<ZapIcon className="size-3 shrink-0" />
											<span className="truncate">{part.skill_name}</span>
										</div>
									);
									if (!part.skill_description) {
										return <div key={part.skill_name}>{row}</div>;
									}
									return (
										<Tooltip key={part.skill_name}>
											<TooltipTrigger asChild>
												<div className="cursor-default">{row}</div>
											</TooltipTrigger>
											<TooltipContent
												side="right"
												sideOffset={4}
												className="max-w-48 text-xs"
											>
												{part.skill_description}
											</TooltipContent>
										</Tooltip>
									);
								})}
							</TooltipProvider>
						</div>
					)}
				</div>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<svg
				className={cn("size-icon-sm -rotate-90", toneClassName)}
				viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
				aria-hidden
			>
				<circle
					cx={RING_SIZE / 2}
					cy={RING_SIZE / 2}
					r={RING_RADIUS}
					fill="none"
					strokeWidth={RING_STROKE}
					className="stroke-content-secondary/25"
				/>
				<circle
					cx={RING_SIZE / 2}
					cy={RING_SIZE / 2}
					r={RING_RADIUS}
					fill="none"
					strokeWidth={RING_STROKE}
					strokeLinecap="round"
					className="stroke-current transition-all duration-300 ease-out"
					style={{
						strokeDasharray: `${RING_CIRCUMFERENCE} ${RING_CIRCUMFERENCE}`,
						strokeDashoffset: dashOffset,
					}}
				/>
			</svg>
		</button>
	);

	// On mobile, a tap toggles the popover. On desktop, hover opens
	// it like a dropdown menu and skill descriptions appear as
	// nested tooltips to the right (same pattern as ModelSelector).
	if (isMobileViewport()) {
		return (
			<Popover>
				<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				<PopoverContent side="top" className="w-auto max-w-72 px-3 py-2">
					{panelContent}
				</PopoverContent>
			</Popover>
		);
	}

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<div onMouseEnter={handleMouseEnter} onMouseLeave={scheduleClose}>
					{triggerButton}
				</div>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				className="w-auto max-w-72 px-3 py-2"
				onMouseEnter={cancelClose}
				onMouseLeave={scheduleClose}
				// Prevent the popover from stealing focus, which would
				// interfere with the chat input.
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				{panelContent}
			</PopoverContent>
		</Popover>
	);
};

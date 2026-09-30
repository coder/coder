import { FlameIcon } from "lucide-react";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useTime } from "#/hooks/useTime";
import { isMobileViewport } from "#/utils/mobile";
import {
	type ChatHeat,
	isCacheLikelyExpired,
} from "./ChatConversation/chatHeat";
import { formatTokenCountCompact } from "./ContextUsageIndicator";

const HEAT_LABELS: Record<ChatHeat["label"], string> = {
	cool: "Cool",
	warm: "Warm",
	hot: "Hot",
};

const EXPIRY_CHECK_INTERVAL_MS = 15_000;

// Blends secondary to warning over the first half of the heat range and
// warning to destructive over the second half.
const heatColor = (heat: number): string => {
	const clamped = Math.min(Math.max(heat, 0), 1);
	if (clamped <= 0.5) {
		const stop = Math.round((clamped / 0.5) * 100);
		return `color-mix(in oklab, var(--color-content-secondary), var(--color-content-warning) ${stop}%)`;
	}
	const stop = Math.round(((clamped - 0.5) / 0.5) * 100);
	return `color-mix(in oklab, var(--color-content-warning), var(--color-content-destructive) ${stop}%)`;
};

const formatPercent = (value: number): string => `${Math.round(value * 100)}%`;

type ChatHeatIndicatorProps = {
	heat: ChatHeat;
	isCacheExpired: boolean;
};

export const ChatHeatIndicator: React.FC<ChatHeatIndicatorProps> = ({
	heat,
	isCacheExpired,
}) => {
	const label = HEAT_LABELS[heat.label];
	const ariaLabel = `Chat heat: ${label}, ${formatPercent(heat.heat)}.${
		isCacheExpired ? " Cache likely expired." : ""
	}`;

	const panelContent = (
		<div className="flex max-w-64 flex-col gap-1 text-xs text-content-primary">
			<span className="font-medium">{`Chat heat: ${label} (${formatPercent(heat.heat)})`}</span>
			<span className="text-content-secondary">
				{`Recent cache miss rate: ${formatPercent(heat.missRate)}`}
			</span>
			<span className="text-content-secondary">
				{`Last request: ${formatTokenCountCompact(heat.lastCacheReadTokens)} cached, ${formatTokenCountCompact(heat.lastFreshTokens)} fresh tokens`}
			</span>
			<span className="text-content-secondary">
				Heat rises when recent requests re-send much of the context window
				instead of reading it from the prompt cache.
			</span>
			{isCacheExpired && (
				<span className="text-content-warning">
					{`Cache likely expired. Your next message will write about ${formatTokenCountCompact(heat.lastPromptTokens)} tokens to the cache again.`}
				</span>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<FlameIcon
				className="size-4"
				style={{ color: heatColor(heat.heat) }}
				aria-hidden="true"
			/>
			{isCacheExpired && (
				<span
					aria-hidden="true"
					className="absolute right-1 top-1 size-1.5 rounded-full bg-content-link"
				/>
			)}
		</button>
	);

	if (isMobileViewport()) {
		return (
			<Popover>
				<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				<PopoverContent
					side="top"
					className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto max-w-72 px-3 py-2"
				>
					{panelContent}
				</PopoverContent>
			</Popover>
		);
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>{triggerButton}</TooltipTrigger>
			<TooltipContent side="top" className="px-3 py-2">
				{panelContent}
			</TooltipContent>
		</Tooltip>
	);
};

type LiveChatHeatIndicatorProps = {
	heat: ChatHeat;
	isStreaming: boolean;
};

/**
 * Rechecks cache expiry on a timer so only this leaf rerenders while the
 * chat sits idle.
 */
export const LiveChatHeatIndicator: React.FC<LiveChatHeatIndicatorProps> = ({
	heat,
	isStreaming,
}) => {
	const nowMs = useTime(() => Date.now(), {
		interval: EXPIRY_CHECK_INTERVAL_MS,
		disabled: isStreaming,
	});
	return (
		<ChatHeatIndicator
			heat={heat}
			isCacheExpired={isCacheLikelyExpired(
				heat.lastRequestAt,
				nowMs,
				isStreaming,
			)}
		/>
	);
};

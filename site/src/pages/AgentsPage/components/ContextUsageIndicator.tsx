import { TriangleAlertIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";
import { isMobileViewport } from "#/utils/mobile";
import {
	ContextDetailsDialog,
	ContextSyncStatus,
} from "./ContextDetailsDialog";
import {
	type AgentContextUsage,
	countLabel,
	formatContextUsageLine,
	formatTokenCount,
	getCompactionThresholdPercent,
	getPercentUsed,
	normalizeContextResources,
} from "./contextResources";
import { SvgRingProgress } from "./SvgRingProgress";

export type { AgentContextUsage } from "./contextResources";

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

const RING_SIZE = 18;
const RING_STROKE = 2.5;

// Delay before the popover closes after the mouse leaves, giving
// the user time to move into the popover content.
const HOVER_CLOSE_DELAY_MS = 150;

export const ContextUsageIndicator: FC<{
	usage: AgentContextUsage | null;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, isRefreshingContext }) => {
	const [open, setOpen] = useState(false);
	// Whether the full-list details dialog is open. The popover and the dialog
	// are mutually exclusive: opening the dialog closes the popover and blocks
	// hover from re-opening it underneath the dialog.
	const [detailsOpen, setDetailsOpen] = useState(false);
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
		if (!detailsOpen) {
			setOpen(true);
		}
	};

	// Every popover-open path is gated on the dialog being closed so hover or
	// tap events firing while the dialog opens cannot re-open the popover
	// underneath it.
	const handlePopoverOpenChange = (nextOpen: boolean) => {
		if (nextOpen && detailsOpen) {
			return;
		}
		setOpen(nextOpen);
	};

	const percentUsed = getPercentUsed(usage);
	const hasPercent = percentUsed !== null;
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 100;
	const toneClassName = getIndicatorToneClassName(percentUsed);
	const compactionThreshold = getCompactionThresholdPercent(usage);

	const context = usage?.context;
	const isDirty = context?.dirty ?? false;
	const contextError = context?.error ?? "";
	const hasContextError = contextError !== "";

	const {
		fileItems,
		skillItems,
		mcpConfigItems,
		mcpServerItems,
		mcpToolCount,
		issueItems,
		hasResources,
	} = normalizeContextResources(context?.resources);

	// Compact count summaries. Zero-count segments are omitted, and the "MCP"
	// prefix attaches to the first MCP segment present.
	const fileSkillSegments: string[] = [];
	if (fileItems.length > 0) {
		fileSkillSegments.push(countLabel(fileItems.length, "context file"));
	}
	if (skillItems.length > 0) {
		fileSkillSegments.push(countLabel(skillItems.length, "skill"));
	}
	const mcpSegments: string[] = [];
	if (mcpConfigItems.length > 0) {
		mcpSegments.push(countLabel(mcpConfigItems.length, "MCP config"));
	}
	if (mcpServerItems.length > 0) {
		mcpSegments.push(
			countLabel(
				mcpServerItems.length,
				mcpConfigItems.length > 0 ? "server" : "MCP server",
			),
		);
	}
	if (mcpToolCount > 0) {
		mcpSegments.push(countLabel(mcpToolCount, "tool"));
	}

	// Whether the details dialog has anything to show. When false, the link
	// row is hidden and clicking the ring is a no-op.
	const hasDetails = hasResources || isDirty || hasContextError;

	const openDetails = () => {
		if (!hasDetails) {
			return;
		}
		cancelClose();
		setOpen(false);
		setDetailsOpen(true);
	};

	const ariaLabelParts = [
		hasPercent
			? `Context usage ${percentLabel}. ${formatTokenCount(usage?.usedTokens)} of ${formatTokenCount(usage?.contextLimitTokens)} tokens used.`
			: "Context usage.",
	];
	if (isDirty) {
		ariaLabelParts.push("Context changed.");
	}
	if (hasDetails) {
		ariaLabelParts.push("Click to open context details.");
	}
	const ariaLabel = ariaLabelParts.join(" ");

	const panelContent = (
		<div className="text-xs text-content-primary">
			{formatContextUsageLine(usage)}
			{compactionThreshold !== null && (
				<div className="mt-1 text-content-secondary">
					{`Compacts at ${compactionThreshold}%`}
				</div>
			)}
			{(fileSkillSegments.length > 0 || mcpSegments.length > 0) && (
				<div className="mt-2 flex flex-col gap-0.5 text-content-secondary">
					{fileSkillSegments.length > 0 && (
						<div>{fileSkillSegments.join(" · ")}</div>
					)}
					{mcpSegments.length > 0 && <div>{mcpSegments.join(" · ")}</div>}
				</div>
			)}
			{issueItems.length > 0 && (
				<div className="mt-2 flex items-center gap-1.5 font-medium text-content-warning">
					<TriangleAlertIcon className="size-3 shrink-0" />
					{countLabel(issueItems.length, "issue")}
				</div>
			)}
			{hasDetails && (
				<button
					type="button"
					onClick={openDetails}
					className="mt-2 block cursor-pointer border-0 bg-transparent p-0 text-left text-xs text-content-link hover:underline"
				>
					Click to open full list
				</button>
			)}
			{(isDirty || hasContextError) && (
				<div className="mt-2 border-0 border-t border-solid border-border-default pt-2">
					<ContextSyncStatus
						contextError={contextError}
						onRefreshContext={onRefreshContext}
						isRefreshingContext={isRefreshingContext}
					/>
				</div>
			)}
		</div>
	);

	const mobile = isMobileViewport();

	// On mobile the tap toggles the popover via PopoverTrigger, so the ring
	// itself gets no click handler there; on desktop clicking the ring opens
	// the details dialog directly.
	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			onClick={mobile ? undefined : openDetails}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<SvgRingProgress
				size={RING_SIZE}
				strokeWidth={RING_STROKE}
				percent={clampedPercent}
				trackClassName="stroke-content-secondary/25"
				progressClassName="stroke-current"
				className={cn("size-icon-sm", toneClassName)}
			/>
			{(isDirty || hasContextError) && (
				<TriangleAlertIcon
					aria-hidden
					className={cn(
						"absolute -right-0.5 -top-0.5 size-3",
						hasContextError
							? "text-content-destructive"
							: "text-content-warning",
					)}
				/>
			)}
		</button>
	);

	// Rendered as a sibling of the popover (never inside its content) so it
	// survives the popover closing.
	const detailsDialog = usage && hasDetails && (
		<ContextDetailsDialog
			usage={usage}
			open={detailsOpen}
			onOpenChange={setDetailsOpen}
			onRefreshContext={onRefreshContext}
			isRefreshingContext={isRefreshingContext}
		/>
	);

	// On mobile, a tap toggles the popover and the link row opens the details
	// dialog. On desktop, hover opens the popover like a dropdown menu and
	// clicking the ring (or the link row) opens the details dialog.
	if (mobile) {
		return (
			<>
				<Popover open={open} onOpenChange={handlePopoverOpenChange}>
					<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
					<PopoverContent
						side="top"
						className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto max-w-72 px-3 py-2"
						// Keep focus where it is when the popover closes so it cannot
						// steal focus from the details dialog as it opens.
						onCloseAutoFocus={(e) => e.preventDefault()}
					>
						{panelContent}
					</PopoverContent>
				</Popover>
				{detailsDialog}
			</>
		);
	}

	return (
		<>
			<Popover open={open} onOpenChange={handlePopoverOpenChange}>
				{/* An anchor rather than a trigger: hover alone controls the
				    popover, leaving click free to open the details dialog. */}
				<PopoverAnchor asChild>
					<div onMouseEnter={handleMouseEnter} onMouseLeave={scheduleClose}>
						{triggerButton}
					</div>
				</PopoverAnchor>
				<PopoverContent
					side="top"
					className="w-auto max-w-72 px-3 py-2"
					onMouseEnter={cancelClose}
					onMouseLeave={scheduleClose}
					// Prevent the popover from stealing focus, which would
					// interfere with the chat input.
					onOpenAutoFocus={(e) => e.preventDefault()}
					// Keep focus where it is when the popover closes so it cannot
					// steal focus from the details dialog as it opens.
					onCloseAutoFocus={(e) => e.preventDefault()}
				>
					{panelContent}
				</PopoverContent>
			</Popover>
			{detailsDialog}
		</>
	);
};

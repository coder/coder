import { TriangleAlertIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useIsMobileViewport } from "#/hooks/useIsMobileViewport";
import { cn } from "#/utils/cn";
import {
	ContextDetailsDialog,
	ContextSyncStatus,
} from "./ContextDetailsDialog";
import {
	type AgentContextUsage,
	countLabel,
	formatContextUsageLine,
	formatTokenCount,
	getCompressionThresholdPercent,
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
	const mobile = useIsMobileViewport();
	const [open, setOpen] = useState(false);
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
		setOpen(true);
	};

	const percentUsed = getPercentUsed(usage);
	const hasPercent = percentUsed !== null;
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 100;
	const toneClassName = getIndicatorToneClassName(percentUsed);
	const compressionThreshold = getCompressionThresholdPercent(usage);

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
	const fileSkillSegments = [
		fileItems.length > 0 && countLabel(fileItems.length, "context file"),
		skillItems.length > 0 && countLabel(skillItems.length, "skill"),
	].filter((segment) => segment !== false);
	const mcpSegments = [
		mcpConfigItems.length > 0 &&
			countLabel(mcpConfigItems.length, "MCP config"),
		mcpServerItems.length > 0 &&
			countLabel(
				mcpServerItems.length,
				mcpConfigItems.length > 0 ? "server" : "MCP server",
			),
		mcpToolCount > 0 && countLabel(mcpToolCount, "tool"),
	].filter((segment) => segment !== false);

	// Whether the details dialog has anything to show. When false, the link
	// row is hidden and clicking the ring is a no-op.
	const hasDetails = hasResources || isDirty || hasContextError;

	// Reset stale dialog state when the details disappear (for example while
	// switching chats), so the next chat's details cannot pop the dialog open
	// without user action.
	if (detailsOpen && !(usage && hasDetails)) {
		setDetailsOpen(false);
	}

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
	// On mobile a tap opens the popover, not the dialog, so the click hint
	// only applies to desktop.
	if (hasDetails && !mobile) {
		ariaLabelParts.push("Click to open context details.");
	}
	const ariaLabel = ariaLabelParts.join(" ");

	const panelContent = (
		<div className="text-xs text-content-primary">
			{formatContextUsageLine(usage)}
			{compressionThreshold !== null && (
				<div className="mt-1 text-content-secondary">
					{`Compacts at ${compressionThreshold}%`}
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

	return (
		<>
			<Popover open={open} onOpenChange={setOpen}>
				{mobile ? (
					<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				) : (
					// An anchor rather than a trigger: hover alone controls the
					// popover, leaving click free to open the details dialog.
					<PopoverAnchor asChild>
						<div onMouseEnter={handleMouseEnter} onMouseLeave={scheduleClose}>
							{triggerButton}
						</div>
					</PopoverAnchor>
				)}
				<PopoverContent
					side="top"
					className={cn(
						"w-auto max-w-72 px-3 py-2",
						mobile &&
							"mobile-full-width-dropdown mobile-full-width-dropdown-bottom",
					)}
					onMouseEnter={mobile ? undefined : cancelClose}
					onMouseLeave={mobile ? undefined : scheduleClose}
					// Prevent the popover from stealing focus, which would
					// interfere with the chat input.
					onOpenAutoFocus={mobile ? undefined : (e) => e.preventDefault()}
					// On desktop, keep focus where it is when the popover closes so
					// it cannot steal focus from the details dialog as it opens. On
					// mobile, let Radix restore focus to the trigger.
					onCloseAutoFocus={mobile ? undefined : (e) => e.preventDefault()}
				>
					{panelContent}
				</PopoverContent>
			</Popover>
			{detailsDialog}
		</>
	);
};

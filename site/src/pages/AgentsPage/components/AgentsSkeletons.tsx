import type { FC } from "react";
import { cn } from "utils/cn";
import { Skeleton } from "#/components/Skeleton/Skeleton";

/** localStorage keys shared with the agents panel components. */
const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";
const RIGHT_PANEL_WIDTH_KEY = "agents.right-panel-width";
const DEFAULT_PANEL_WIDTH = 480;
const MIN_PANEL_WIDTH = 360;

/** Read persisted right-panel state for use in static skeletons. */
function getRightPanelState(): { open: boolean; width: number } {
	const open = localStorage.getItem(RIGHT_PANEL_OPEN_KEY) === "true";
	const stored = localStorage.getItem(RIGHT_PANEL_WIDTH_KEY);
	let width = DEFAULT_PANEL_WIDTH;
	if (stored) {
		const parsed = Number.parseInt(stored, 10);
		if (!Number.isNaN(parsed) && parsed >= MIN_PANEL_WIDTH) {
			width = parsed;
		}
	}
	return { open, width };
}

/**
 * Skeleton shown while the AgentsPage chunk is loading. Mimics the
 * sidebar + empty main area layout so the user sees structure
 * immediately instead of a fullscreen spinner.
 */
export const AgentsPageSkeleton: FC = () => (
	<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
		<div className="order-2 md:order-none flex-1 min-h-0 border-t border-border-default md:flex-none md:border-t-0 md:h-full md:w-[320px] md:min-h-0 md:border-b-0">
			<div className="relative flex h-full w-full min-h-0 border-0 border-r border-solid overflow-hidden">
				<div className="absolute inset-0 flex flex-col">
					<div className="hidden border-b border-border-default px-2 pb-3 pt-1.5 md:block">
						<div className="mb-2.5 flex items-center justify-between">
							<Skeleton className="h-6 w-6 rounded" />
							<div className="flex items-center gap-0.5 -mr-1.5">
								<Skeleton className="h-7 w-7 rounded" />
								<Skeleton className="h-7 w-7 rounded" />
								<Skeleton className="h-7 w-7 rounded" />
							</div>
						</div>
						<Skeleton className="h-9 w-full rounded-md" />
					</div>
					<div className="flex flex-col gap-2 px-2 py-3">
						<Skeleton className="ml-2.5 h-3.5 w-16" />
						<div className="flex flex-col gap-0.5">
							{Array.from({ length: 6 }, (_, i) => (
								<div
									key={i}
									className="flex items-start gap-2 rounded-md px-2 py-1"
								>
									<Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded-md" />
									<div className="min-w-0 flex-1 space-y-1.5">
										<Skeleton
											className="h-3.5"
											style={{ width: `${55 + ((i * 17) % 35)}%` }}
										/>
										<Skeleton className="h-3 w-20" />
									</div>
								</div>
							))}
						</div>
					</div>
				</div>
			</div>
		</div>
		<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary order-1 md:order-none" />
	</div>
);

/**
 * Skeleton placeholder for a chat conversation: two user message
 * bubbles interleaved with assistant response lines.
 */
export const ChatConversationSkeleton: FC = () => (
	<div className="flex flex-col gap-3">
		{/* User message bubble (right-aligned) */}
		<div className="flex w-full justify-end">
			<Skeleton className="h-10 w-2/3 rounded-lg" />
		</div>
		{/* Assistant response lines (left-aligned) */}
		<div className="space-y-3">
			<Skeleton className="h-4 w-full" />
			<Skeleton className="h-4 w-5/6" />
			<Skeleton className="h-4 w-4/6" />
		</div>
		{/* Second user message bubble */}
		<div className="mt-3 flex w-full justify-end">
			<Skeleton className="h-10 w-1/2 rounded-lg" />
		</div>
		{/* Second assistant response */}
		<div className="space-y-3">
			<Skeleton className="h-4 w-full" />
			<Skeleton className="h-4 w-5/6" />
			<Skeleton className="h-4 w-4/6" />
			<Skeleton className="h-4 w-full" />
			<Skeleton className="h-4 w-3/5" />
		</div>
	</div>
);

/**
 * Skeleton placeholder for the right sidebar panel: a tab bar and
 * a few content lines.
 */
export const RightPanelSkeleton: FC = () => (
	<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
		{/* Skeleton tab bar */}
		<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1">
			<Skeleton className="h-6 w-12 rounded-md" />
			<div className="flex-1" />
		</div>
		{/* Skeleton panel content */}
		<div className="space-y-4 p-4">
			<Skeleton className="h-4 w-32" />
			<Skeleton className="h-3 w-full" />
			<Skeleton className="h-3 w-3/4" />
		</div>
	</div>
);

/**
 * Skeleton placeholder for the chat input area. Matches the layout of
 * the real AgentChatInput so the transition from Suspense fallback to
 * the loaded component doesn't cause a vertical layout shift.
 */
const ChatInputSkeleton: FC = () => (
	<div className="shrink-0 overflow-y-auto px-4 [scrollbar-gutter:stable] [scrollbar-width:thin]">
		<div className="mx-auto w-full max-w-3xl pb-0 sm:pb-4">
			<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm">
				<div className="min-h-[60px] sm:min-h-24 px-3 py-2" />
				<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
					<Skeleton className="h-6 w-24 rounded" />
					<Skeleton className="size-7 rounded-full" />
				</div>
			</div>
		</div>
	</div>
);

/**
 * Skeleton shown while the AgentDetail chunk is loading. Mimics a
 * top bar + chat conversation layout so the user sees navigable
 * structure during the brief Suspense fallback.
 */
export const AgentDetailSkeleton: FC = () => {
	const rightPanel = getRightPanelState();

	return (
		<div
			className={cn(
				"relative flex h-full min-h-0 min-w-0 flex-1",
				rightPanel.open && "flex-row",
			)}
		>
			<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
				<div className="flex shrink-0 items-center gap-2 px-4 py-1.5">
					<Skeleton className="h-7 w-7 rounded" />
					<Skeleton className="h-4 w-32" />
					<div className="flex-1" />
					<Skeleton className="h-7 w-7 rounded" />
					<Skeleton className="h-7 w-7 rounded" />
				</div>
				<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
					<div className="px-4">
						<div className="mx-auto w-full max-w-3xl py-6">
							<ChatConversationSkeleton />
						</div>
					</div>
				</div>
				<ChatInputSkeleton />
			</div>
			{rightPanel.open && (
				<div
					style={
						{
							"--panel-width": `${rightPanel.width}px`,
						} as React.CSSProperties
					}
					className="relative flex h-full w-[100vw] min-w-0 flex-col border-0 border-l border-solid border-border-default sm:w-[var(--panel-width)] sm:min-w-[360px] sm:max-w-[70vw]"
				>
					<RightPanelSkeleton />
				</div>
			)}
		</div>
	);
};

import { CalendarClockIcon, WebhookIcon } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Link as RouterLink } from "react-router";
import type {
	ChatAutomation,
	ChatAutomationPausedReason,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Link, type LinkProps } from "#/components/Link/Link";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatDate } from "#/utils/time";
import { describeSchedule } from "./AutomationScheduleFields";

type ChatTitleLinkProps = {
	chatId: string;
	title: string | undefined;
	size?: LinkProps["size"];
	className?: string;
};

/** Links to a chat by its title, falling back to "Untitled". */
export const ChatTitleLink: React.FC<ChatTitleLinkProps> = ({
	chatId,
	title,
	size = "lg",
	className,
}) => {
	return (
		<Link asChild showExternalIcon={false} size={size} className={className}>
			<RouterLink to={`/agents/${chatId}`}>{title || "Untitled"}</RouterLink>
		</Link>
	);
};

const pausedReasonText: Record<ChatAutomationPausedReason, string> = {
	experiment_disabled: "The owner no longer has the automations experiment.",
	model_unavailable:
		"Its model is no longer available. The owner must choose another model.",
	owner_inactive: "Its owner is suspended or inactive.",
	owner_not_member: "Its owner left the organization.",
	target_unavailable:
		"Its chat is archived or deleted. Unarchive the chat, or create a new automation.",
};

/** Why the server refuses to run this automation, one sentence per reason. */
const pausedReasons = (automation: ChatAutomation): string[] =>
	(automation.paused_reasons ?? []).map((reason) => pausedReasonText[reason]);

const isUsedSingleUseWebhook = (automation: ChatAutomation) =>
	automation.webhook_use === "single" &&
	Boolean(automation.webhook_consumed_at);

/** True when an existing-chat automation has no chat it can run in. */
export const isTargetMissing = (automation: ChatAutomation) =>
	automation.target_mode === "existing_chat" &&
	(!automation.target_chat_id ||
		Boolean(automation.target_chat?.archived) ||
		Boolean(automation.paused_reasons?.includes("target_unavailable")));

/** Why Run now cannot start a run, or undefined when it can. */
export const runNowBlockedReason = (
	automation: ChatAutomation,
): string | undefined => {
	if (!automation.enabled) {
		return "Turn the automation on to run it.";
	}
	const reasons = pausedReasons(automation);
	if (reasons.length > 0) {
		return reasons.join(" ");
	}
	if (isTargetMissing(automation)) {
		return pausedReasonText.target_unavailable;
	}
	return undefined;
};

type AutomationPausedBadgeProps = {
	automation: ChatAutomation;
};

/** A Paused badge whose tooltip says why the server refuses runs. */
export const AutomationPausedBadge: React.FC<AutomationPausedBadgeProps> = ({
	automation,
}) => {
	const reasons = pausedReasons(automation);
	if (reasons.length === 0) {
		return null;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge
					variant="warning"
					size="sm"
					tabIndex={0}
					role="note"
					aria-label={`Paused: ${reasons.join(" ")}`}
				>
					Paused
				</Badge>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">
				{reasons.map((reason) => (
					<div key={reason}>{reason}</div>
				))}
			</TooltipContent>
		</Tooltip>
	);
};

type AutomationTriggerSummaryProps = {
	automation: ChatAutomation;
};

/** The trigger in words, such as "Weekdays at 09:00" over "UTC". */
export const AutomationTriggerSummary: React.FC<
	AutomationTriggerSummaryProps
> = ({ automation }) => {
	const isSchedule = automation.kind === "schedule";
	const Icon = isSchedule ? CalendarClockIcon : WebhookIcon;
	return (
		<div className="flex items-start gap-2">
			<Icon
				aria-hidden
				className="mt-0.5 size-4 shrink-0 text-content-secondary"
			/>
			<div className="flex min-w-0 flex-col gap-0.5">
				<span className="text-content-primary">
					{isSchedule
						? describeSchedule(automation.schedule_cron ?? "")
						: "Webhook"}
				</span>
				<span className="text-xs text-content-secondary">
					{isSchedule
						? automation.schedule_time_zone || "UTC"
						: automation.webhook_use === "single"
							? "Single-use"
							: "Multi-use"}
				</span>
			</div>
		</div>
	);
};

/** Returns the first run after now, or undefined when none is scheduled. */
const findNextRun = (
	automation: ChatAutomation,
	now: number,
): Date | undefined => {
	if (automation.kind !== "schedule") {
		return undefined;
	}
	// The server computed the list when it last loaded, so its first entry
	// can already be in the past.
	return automation.next_run_times
		.map((runTime) => new Date(runTime))
		.find((runTime) => runTime.getTime() > now);
};

// setTimeout fires at once for delays above this.
const maxTimeoutDelay = 2 ** 31 - 1;

type ScheduledRunProps = {
	automation: ChatAutomation;
};

const ScheduledRun: React.FC<ScheduledRunProps> = ({ automation }) => {
	const [now, setNow] = useState(Date.now);
	const nextRun = findNextRun(automation, now);
	const nextRunTime = nextRun?.getTime();
	// One timer per row, fired when the shown run starts, instead of polling.
	useEffect(() => {
		if (nextRunTime === undefined) {
			return;
		}
		const delay = nextRunTime - Date.now();
		// Runs this far out need no timer while the page is open.
		if (delay > maxTimeoutDelay) {
			return;
		}
		// Never earlier than the run, even if the timer fires a little early.
		const handle = setTimeout(
			() => setNow(Math.max(Date.now(), nextRunTime)),
			Math.max(delay, 0),
		);
		return () => clearTimeout(handle);
	}, [nextRunTime]);
	if (!nextRun) {
		return "Not scheduled";
	}
	return formatDate(nextRun, {
		locale: "en-US",
		timeZone: automation.schedule_time_zone || "UTC",
		timeZoneName: "short",
		month: "short",
		day: "numeric",
		year: undefined,
		hour: "numeric",
		minute: "2-digit",
		second: undefined,
	});
};

type AutomationNextRunProps = {
	automation: ChatAutomation;
};

/**
 * What happens next: the next scheduled run, Listening for a webhook, or why
 * nothing will run (Used, Disabled, Paused).
 */
export const AutomationNextRun: React.FC<AutomationNextRunProps> = ({
	automation,
}) => {
	if (isUsedSingleUseWebhook(automation)) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						aria-label="Used. A single-use webhook accepts one event. Create a new automation to receive another."
						className="cursor-default border-none bg-transparent p-0 text-content-secondary"
					>
						Used
					</button>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					A single-use webhook accepts one event. Create a new automation to
					receive another.
				</TooltipContent>
			</Tooltip>
		);
	}
	if (!automation.enabled) {
		return <span className="text-content-secondary">Disabled</span>;
	}
	if (pausedReasons(automation).length > 0) {
		return <AutomationPausedBadge automation={automation} />;
	}
	if (automation.kind === "webhook") {
		return <span className="text-content-primary">Listening</span>;
	}
	return <ScheduledRun automation={automation} />;
};

type AutomationEnabledSwitchProps = {
	automation: ChatAutomation;
	isOwner: boolean;
	isUpdating: boolean;
	onToggle: (enabled: boolean) => void;
	/** Shows "Enabled" or "Disabled" next to the switch. */
	showLabel?: boolean;
};

/**
 * The owner can turn an automation on and off. Other viewers with access,
 * such as admins, can only turn it off. The switch explains in a tooltip why
 * it does not move, and stays focusable so keyboard users can read that.
 */
export const AutomationEnabledSwitch: React.FC<
	AutomationEnabledSwitchProps
> = ({ automation, isOwner, isUpdating, onToggle, showLabel = false }) => {
	const reasonId = useId();
	// The switch shows the server's enabled flag. A used single-use webhook
	// stays on and its Next run reads Used; turning it back on cannot re-arm it.
	const checked = automation.enabled;
	let blockedReason: string | undefined;
	if (isUsedSingleUseWebhook(automation) && !automation.enabled) {
		blockedReason = "A used single-use webhook cannot run again.";
	} else if (!isOwner && !automation.enabled) {
		blockedReason = "Only the owner can turn this automation back on.";
	}
	const control = (
		<Switch
			checked={checked}
			disabled={isUpdating}
			aria-disabled={blockedReason !== undefined}
			aria-describedby={blockedReason ? reasonId : undefined}
			aria-label={`Enable ${automation.name}`}
			// A blocked off switch keeps a visible outline, so the cell does
			// not look empty on a dark row.
			className="aria-disabled:cursor-not-allowed aria-disabled:data-[state=unchecked]:border-border"
			onCheckedChange={(enabled) => {
				if (!blockedReason) {
					onToggle(enabled);
				}
			}}
		/>
	);
	return (
		<span className="inline-flex items-center gap-2">
			{blockedReason ? (
				<Tooltip>
					{/* A span trigger: asChild on the switch would replace its
					    data-state with the tooltip's, which drops the switch colors. */}
					<TooltipTrigger asChild>
						<span className="inline-flex">{control}</span>
					</TooltipTrigger>
					<TooltipContent>{blockedReason}</TooltipContent>
				</Tooltip>
			) : (
				control
			)}
			{blockedReason && (
				<span id={reasonId} className="sr-only">
					{blockedReason}
				</span>
			)}
			{showLabel && (
				<span className="text-sm text-content-primary">
					{checked ? "Enabled" : "Disabled"}
				</span>
			)}
		</span>
	);
};

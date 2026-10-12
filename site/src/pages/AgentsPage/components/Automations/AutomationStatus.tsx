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
	model_unavailable: "Its model is no longer available.",
	owner_inactive: "Its owner is suspended or inactive.",
	target_unavailable: "Its chat is archived or deleted.",
};

export const isUsedSingleUseWebhook = (automation: ChatAutomation) =>
	automation.webhook_use === "single" &&
	Boolean(automation.webhook_consumed_at);

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
		return <span className="text-content-secondary">Used</span>;
	}
	if (!automation.enabled) {
		return <span className="text-content-secondary">Disabled</span>;
	}
	const pausedReasons = automation.paused_reasons ?? [];
	if (pausedReasons.length > 0) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge
						variant="warning"
						size="sm"
						tabIndex={0}
						role="note"
						aria-label={`Paused: ${pausedReasons.map((reason) => pausedReasonText[reason]).join(" ")}`}
					>
						Paused
					</Badge>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					{pausedReasons.map((reason) => (
						<div key={reason}>{pausedReasonText[reason]}</div>
					))}
				</TooltipContent>
			</Tooltip>
		);
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
	const isUsed = isUsedSingleUseWebhook(automation);
	// A used single-use webhook rejects every delivery, so it shows as off.
	const checked = automation.enabled && !isUsed;
	let blockedReason: string | undefined;
	if (isUsed) {
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
			className="aria-disabled:cursor-not-allowed aria-disabled:data-[state=unchecked]:bg-surface-tertiary"
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
					<TooltipTrigger asChild>{control}</TooltipTrigger>
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

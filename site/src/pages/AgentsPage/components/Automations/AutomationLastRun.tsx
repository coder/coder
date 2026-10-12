import { cn } from "cn";
import { Link as RouterLink } from "react-router";
import type {
	ChatAutomationLastRun,
	ChatAutomationRunOutcome,
	ChatAutomationRunReason,
} from "#/api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatDate, timeFrom } from "#/utils/time";

type ReasonText = {
	/** Short label after the outcome, such as "chat busy". */
	label: string;
	/** What the owner should do. Set for refused runs only. */
	hint?: string;
};

/**
 * Words for every run reason the server sends. Skipped reasons resolve on
 * their own; refused reasons need someone to change something.
 */
// previous_run_active ships in the next API change; listing it here keeps
// the map complete once the generated type adds it.
type KnownRunReason = ChatAutomationRunReason | "previous_run_active";

export const runReasonText: Record<KnownRunReason, ReasonText> = {
	chat_busy: { label: "chat busy" },
	queue_full: { label: "queue full" },
	missed: { label: "missed" },
	previous_run_active: { label: "previous run still running" },
	hook_denied: {
		label: "hook denied",
		hint: "A hook blocked the run. Check your hook service.",
	},
	owner_inactive: {
		label: "owner inactive",
		hint: "The owner is suspended or dormant. An admin must reactivate them.",
	},
	forbidden: {
		label: "not allowed",
		hint: "The owner lost access to the chat or the organization.",
	},
	model_unavailable: {
		label: "model unavailable",
		hint: "The model is turned off or was removed. Pick another model.",
	},
	target_unavailable: {
		label: "chat unavailable",
		hint: "The chat is archived or deleted. Unarchive it or pick another chat.",
	},
	experiment_disabled: {
		label: "automations off",
		hint: "The automations experiment is off for the owner.",
	},
	disabled: {
		label: "automation off",
		hint: "The automation was off when the event arrived. Turn it on to accept events.",
	},
	single_use_consumed: {
		label: "already used",
		hint: "A single-use webhook accepts one event. Create a new automation for the next one.",
	},
	body_too_large: {
		label: "body too large",
		hint: "The webhook body was larger than the deployment limit. Send a smaller body.",
	},
	invalid_body: {
		label: "invalid body",
		hint: "The webhook body was not valid JSON. Fix the sender.",
	},
	no_default_model: {
		label: "no default model",
		hint: "No default model is configured. Pick a model or ask an admin to set a default.",
	},
	error: {
		label: "server error",
		hint: "The server could not start the run. Check the server logs, or try Run now.",
	},
};

/** A last run whose reason can be one this UI does not know yet. */
export type AutomationLastRunValue = Omit<ChatAutomationLastRun, "reason"> & {
	reason?: string;
};

const outcomeLabel: Record<ChatAutomationRunOutcome, string> = {
	accepted: "Accepted",
	skipped: "Skipped",
	refused: "Refused",
};

/** Returns the words for a reason, with a fallback for values this UI does not know yet. */
export const describeRunReason = (reason: string): ReasonText => {
	const known = (runReasonText as Record<string, ReasonText | undefined>)[
		reason
	];
	return known ?? { label: reason.replaceAll("_", " ") };
};

/** "Accepted", "Skipped: chat busy" or "Refused: hook denied". */
export const describeLastRunOutcome = (
	lastRun: AutomationLastRunValue,
): string => {
	const outcome = outcomeLabel[lastRun.outcome] ?? lastRun.outcome;
	return lastRun.reason
		? `${outcome}: ${describeRunReason(lastRun.reason).label}`
		: outcome;
};

const formatAbsolute = (at: string) =>
	formatDate(new Date(at), {
		locale: "en-US",
		timeZoneName: "short",
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "numeric",
		minute: "2-digit",
		second: undefined,
	});

type AutomationLastRunProps = {
	lastRun: AutomationLastRunValue | null;
	/** The current time; a prop so stories and tests render the same text. */
	now?: Date;
	/** Adds the owner's next step for a refused run. */
	showHint?: boolean;
	className?: string;
};

/**
 * One line such as "Last run 5 minutes ago · Skipped: chat busy", with the
 * exact time in a tooltip and a link to the chat that received the run.
 */
export const AutomationLastRun: React.FC<AutomationLastRunProps> = ({
	lastRun,
	now,
	showHint = false,
	className,
}) => {
	if (!lastRun) {
		return (
			<span className={cn("text-xs text-content-secondary", className)}>
				No runs yet
			</span>
		);
	}
	const isRefused = lastRun.outcome === "refused";
	const hint =
		showHint && isRefused && lastRun.reason
			? describeRunReason(lastRun.reason).hint
			: undefined;
	return (
		<span className={cn("flex flex-col gap-0.5 text-xs", className)}>
			<span className="text-content-secondary">
				Last run{" "}
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							className="cursor-default border-none bg-transparent p-0 text-inherit underline decoration-dotted underline-offset-2"
						>
							{timeFrom(lastRun.at, now ?? Date.now())}
						</button>
					</TooltipTrigger>
					<TooltipContent>{formatAbsolute(lastRun.at)}</TooltipContent>
				</Tooltip>
				{" · "}
				<span
					className={cn(
						isRefused ? "text-content-warning" : "text-content-secondary",
						lastRun.outcome === "accepted" && "text-content-primary",
					)}
				>
					{describeLastRunOutcome(lastRun)}
				</span>
				{lastRun.chat_id && (
					<>
						{" · "}
						<RouterLink
							to={`/agents/${lastRun.chat_id}`}
							aria-label="Open chat of the last run"
							className="text-content-link no-underline hover:underline"
						>
							Open chat
						</RouterLink>
					</>
				)}
			</span>
			{hint && <span className="text-content-warning">{hint}</span>}
		</span>
	);
};

import { cn } from "cn";
import {
	CornerDownRightIcon,
	EllipsisIcon,
	LinkIcon,
	MessagesSquareIcon,
	PlayIcon,
	ShieldCheckIcon,
	ShieldIcon,
	Trash2Icon,
	TriangleAlertIcon,
} from "lucide-react";
import { useId, useRef } from "react";
import type {
	ChatProject,
	ChatProjectTrigger,
	UpdateChatProjectTriggerRequest,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { DATE_FORMAT, timeFrom } from "#/utils/time";
import { SettingsCardRow, SettingsCardRowIcon } from "./ProjectSection";
import {
	BusyChip,
	ConversationChip,
	EventsChip,
	PromptChip,
	ScheduleChip,
	ScopeChip,
	SendersChip,
	SlackMessageChip,
	TextPatternChip,
	TriggerSecretChip,
} from "./TriggerChips";
import { TriggerDeliverySummary } from "./TriggerDeliveries";
import type { TriggerUpdateCallbacks } from "./TriggerSentenceChip";
import { TriggerSourceIcon } from "./TriggerSourceIcon";
import {
	hasUnverifiedFilters,
	triggerSecrets,
	triggerSources,
} from "./triggerCatalog";
import { formatInTimeZone } from "./triggerSchedule";

type TriggerRowProps = {
	readonly trigger: ChatProjectTrigger;
	readonly project: ChatProject;
	/** False shows the trigger read-only. */
	readonly canManage: boolean;
	/** True while a Run now of this trigger is starting. */
	readonly isRunning: boolean;
	/** The time relative times count from. */
	readonly now: Date;
	/** The viewer's time zone, which exact times are shown in. */
	readonly timeZone: string;
	readonly onUpdate: (
		trigger: ChatProjectTrigger,
		request: UpdateChatProjectTriggerRequest,
		callbacks?: TriggerUpdateCallbacks,
	) => void;
	readonly onRegenerateURL: (
		trigger: ChatProjectTrigger,
		restoreFocus: () => void,
	) => void;
	/** Runs a schedule trigger outside its schedule. */
	readonly onRunNow: (trigger: ChatProjectTrigger) => void;
	readonly onDelete: (
		trigger: ChatProjectTrigger,
		restoreFocus: () => void,
	) => void;
};

/** "Last triggered 2 hours ago", or "Never ran" for a schedule. */
const lastRunLabel = (trigger: ChatProjectTrigger, now: Date): string => {
	const { lastRunPrefix, neverRan } = triggerSources[trigger.source];
	return trigger.last_triggered_at
		? `${lastRunPrefix} ${timeFrom(trigger.last_triggered_at, now)}`
		: neverRan;
};

type ScheduleNextRunProps = {
	/** Unset while the trigger is off. */
	readonly nextRunAt: string | undefined;
	readonly now: Date;
	readonly timeZone: string;
};

/** "Next run in 3 hours", with the exact time on hover. */
const ScheduleNextRun: React.FC<ScheduleNextRunProps> = ({
	nextRunAt,
	now,
	timeZone,
}) => {
	if (!nextRunAt) {
		return null;
	}
	return (
		<span className="text-xs">
			Next run{" "}
			<time
				dateTime={nextRunAt}
				title={formatInTimeZone(nextRunAt, timeZone, DATE_FORMAT.FULL_DATETIME)}
			>
				{timeFrom(nextRunAt, now)}
			</time>
		</span>
	);
};

/**
 * One trigger as a sentence of editable chips, with its prompt and its last
 * webhook delivery or run below, an on and off switch, and a menu for the
 * webhook URL or Run now, and deletion.
 */
export const TriggerRow: React.FC<TriggerRowProps> = ({
	trigger,
	project,
	canManage,
	isRunning,
	now,
	timeZone,
	onUpdate,
	onRegenerateURL,
	onRunNow,
	onDelete,
}) => {
	const runNoteId = useId();
	// The menu items unmount on select, so dialogs they open return focus
	// to the menu button instead.
	const actionsRef = useRef<HTMLButtonElement>(null);
	const focusActions = () => actionsRef.current?.focus();
	const secret = triggerSecrets[trigger.source];
	const isSchedule = trigger.source === "schedule";
	// Run now needs the trigger on; the menu says why it is missing.
	const showRunNote = isSchedule && !trigger.enabled;
	const chipProps = {
		trigger,
		readOnly: !canManage,
		onUpdate: (
			request: UpdateChatProjectTriggerRequest,
			callbacks?: TriggerUpdateCallbacks,
		) => onUpdate(trigger, request, callbacks),
	};
	return (
		<SettingsCardRow className="items-start">
			<div
				className={cn(
					"flex min-w-0 flex-1 items-start gap-3",
					!trigger.enabled && "opacity-60",
				)}
			>
				<SettingsCardRowIcon className="mt-0.5">
					<TriggerSourceIcon source={trigger.source} />
				</SettingsCardRowIcon>
				<div className="flex min-w-0 flex-1 flex-col gap-1.5 text-content-secondary">
					<div className="flex flex-wrap items-center gap-1.5">
						{isSchedule ? (
							<>
								<ScheduleChip {...chipProps} now={now} timeZone={timeZone} />
								<ScheduleNextRun
									nextRunAt={trigger.next_run_at}
									now={now}
									timeZone={timeZone}
								/>
							</>
						) : (
							<span>When</span>
						)}
						{trigger.source === "slack" && (
							<>
								<SlackMessageChip {...chipProps} />
								<span>from</span>
								<SendersChip {...chipProps} />
								<span>in</span>
								<ScopeChip {...chipProps} />
							</>
						)}
						{trigger.source === "github" && (
							<>
								<EventsChip {...chipProps} />
								<span>by</span>
								<SendersChip {...chipProps} />
								<span>in</span>
								<ScopeChip {...chipProps} />
								<span>containing</span>
								<TextPatternChip {...chipProps} />
							</>
						)}
						{trigger.source === "webhook" && (
							<>
								<span>this webhook is called with</span>
								<TextPatternChip {...chipProps} />
							</>
						)}
					</div>
					<div className="flex min-w-0 items-center gap-1.5">
						<CornerDownRightIcon
							aria-hidden
							className="size-icon-xs shrink-0"
						/>
						<span className="shrink-0">
							Ask <span className="text-content-primary">{project.name}</span>{" "}
							to
						</span>
						<PromptChip {...chipProps} agentName={project.name} />
					</div>
					<div className="flex flex-wrap items-center gap-1.5">
						<MessagesSquareIcon aria-hidden className="size-icon-xs shrink-0" />
						<span>{triggerSources[trigger.source].followUps}</span>
						<ConversationChip {...chipProps} />
						{trigger.continue_conversations && (
							<>
								<span>and, if {project.name} is busy,</span>
								<BusyChip {...chipProps} agentName={project.name} />
							</>
						)}
					</div>
					{secret && (
						<div className="flex flex-wrap items-center gap-1.5">
							{secret.isSet(trigger) ? (
								<ShieldCheckIcon
									aria-hidden
									className="size-icon-xs shrink-0 text-content-success"
								/>
							) : (
								<ShieldIcon aria-hidden className="size-icon-xs shrink-0" />
							)}
							<span>{secret.title}</span>
							<TriggerSecretChip {...chipProps} secret={secret} />
						</div>
					)}
					{secret && canManage && hasUnverifiedFilters(trigger) && (
						<p className="m-0 flex items-start gap-1.5 text-xs text-content-warning">
							<TriangleAlertIcon
								aria-hidden
								className="mt-0.5 size-icon-xs shrink-0"
							/>
							Without a {secret.label.toLowerCase()}, anyone with the webhook
							URL can send events that pass these filters.
						</p>
					)}
					<TriggerDeliverySummary
						project={project}
						trigger={trigger}
						now={now}
						timeZone={timeZone}
					/>
				</div>
			</div>
			{canManage ? (
				<Switch
					className="mt-1"
					checked={trigger.enabled}
					aria-label={`Enable ${trigger.name}`}
					onCheckedChange={(enabled) => onUpdate(trigger, { enabled })}
				/>
			) : (
				<Badge size="sm" variant="outline" className="mt-1.5">
					{trigger.enabled ? "On" : "Off"}
				</Badge>
			)}
			{canManage && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							ref={actionsRef}
							size="icon"
							variant="subtle"
							aria-label={`More actions for ${trigger.name}`}
						>
							<EllipsisIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						aria-describedby={showRunNote ? runNoteId : undefined}
					>
						<DropdownMenuLabel className="text-xs font-normal">
							{lastRunLabel(trigger, now)}
						</DropdownMenuLabel>
						{showRunNote && (
							<DropdownMenuLabel
								id={runNoteId}
								className="pt-0 text-xs font-normal text-content-secondary"
							>
								Turn the trigger on to run it now.
							</DropdownMenuLabel>
						)}
						<DropdownMenuSeparator />
						{isSchedule ? (
							trigger.enabled && (
								<DropdownMenuItem
									disabled={isRunning}
									onSelect={() => onRunNow(trigger)}
								>
									<Spinner loading={isRunning}>
										<PlayIcon />
									</Spinner>
									Run now
								</DropdownMenuItem>
							)
						) : (
							<DropdownMenuItem
								onSelect={() => onRegenerateURL(trigger, focusActions)}
							>
								<LinkIcon />
								Get new webhook URL
							</DropdownMenuItem>
						)}
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onSelect={() => onDelete(trigger, focusActions)}
						>
							<Trash2Icon />
							Delete
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</SettingsCardRow>
	);
};

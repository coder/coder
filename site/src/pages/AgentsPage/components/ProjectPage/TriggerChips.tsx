import type { FormikProps } from "formik";
import { useId } from "react";
import type {
	ChatProjectTrigger,
	ChatProjectTriggerBusyBehavior,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Switch } from "#/components/Switch/Switch";
import { TagInput } from "#/components/TagInput/TagInput";
import { Textarea } from "#/components/Textarea/Textarea";
import { extractTimezone } from "#/utils/schedule";
import {
	EventChecklist,
	FieldHint,
	SaveError,
	SwitchRow,
	TextPatternFields,
	TriggerSecretFields,
} from "./TriggerChipEditors";
import { TriggerScheduleFields } from "./TriggerScheduleFields";
import {
	SentenceChip,
	type TriggerChipProps,
	useTriggerChipForm,
} from "./TriggerSentenceChip";
import {
	type TriggerSecretInfo,
	triggerEventsLabel,
	triggerNameMaxLength,
	triggerPromptMaxLength,
	triggerSources,
	validateTriggerScope,
	validateTriggerTextPattern,
} from "./triggerCatalog";
import {
	describeSchedule,
	draftFromSchedule,
	scheduleFromDraft,
	validateScheduleDraft,
} from "./triggerSchedule";

/** The GitHub events chip: "[Pull request opened ▾]". */
export const EventsChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => (
	<SentenceChip
		label={triggerEventsLabel(trigger.source, trigger.events)}
		fieldLabel="Events"
		readOnly={readOnly}
	>
		{() => <EventChecklist trigger={trigger} onUpdate={onUpdate} />}
	</SentenceChip>
);

const textPatternLabel = (trigger: ChatProjectTrigger): string => {
	if (!trigger.text_pattern) {
		return "Any text";
	}
	return trigger.text_pattern_is_regex
		? `/${trigger.text_pattern}/`
		: `"${trigger.text_pattern}"`;
};

type TextPatternValues = {
	readonly text: string;
	readonly isRegex: boolean;
};

/**
 * The text filter and its regex switch as one draft, saved together so
 * turning on regex never checks the old text as a regex.
 */
const useTextPatternForm = (
	trigger: ChatProjectTrigger,
	onUpdate: TriggerChipProps["onUpdate"],
) => {
	const saved: TextPatternValues = {
		text: trigger.text_pattern ?? "",
		isRegex: Boolean(trigger.text_pattern_is_regex),
	};
	return useTriggerChipForm({
		saved,
		validate: ({ text, isRegex }) => {
			const error = validateTriggerTextPattern(text.trim(), isRegex);
			return error ? { text: error } : {};
		},
		toRequest: ({ text, isRegex }) => ({
			...(text.trim() !== saved.text && { text_pattern: text.trim() }),
			...(isRegex !== saved.isRegex && { text_pattern_is_regex: isRegex }),
		}),
		onUpdate,
	});
};

type TextPatternEditorProps = {
	readonly form: Pick<
		FormikProps<TextPatternValues>,
		"values" | "setFieldValue"
	>;
	/** Saves the draft and closes the editor. */
	readonly onSubmit: () => void;
};

/** The fields of a `useTextPatternForm` draft. */
const TextPatternEditor: React.FC<TextPatternEditorProps> = ({
	form,
	onSubmit,
}) => {
	const { text, isRegex } = form.values;
	return (
		<TextPatternFields
			text={text}
			isRegex={isRegex}
			error={validateTriggerTextPattern(text.trim(), isRegex)}
			onTextChange={(next) => void form.setFieldValue("text", next)}
			onRegexChange={(next) => void form.setFieldValue("isRegex", next)}
			onSubmit={onSubmit}
		/>
	);
};

/** The text filter chip for GitHub and webhook triggers: "[Any text ▾]". */
export const TextPatternChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => {
	const { form, saveError, chipProps } = useTextPatternForm(trigger, onUpdate);
	return (
		<SentenceChip
			label={textPatternLabel(trigger)}
			fieldLabel="Text filter"
			readOnly={readOnly}
			{...chipProps}
		>
			{(close) => (
				<>
					<SaveError message={saveError} />
					<TextPatternEditor form={form} onSubmit={close} />
				</>
			)}
		</SentenceChip>
	);
};

/** The Slack message chip: events, text filter, and thread replies. */
export const SlackMessageChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => {
	const threadId = useId();
	const { form, saveError, chipProps } = useTextPatternForm(trigger, onUpdate);
	const eventsLabel = triggerEventsLabel(trigger.source, trigger.events);
	return (
		<SentenceChip
			label={
				trigger.text_pattern
					? `${eventsLabel} matching ${textPatternLabel(trigger)}`
					: eventsLabel
			}
			fieldLabel="Messages"
			readOnly={readOnly}
			{...chipProps}
		>
			{(close) => (
				<>
					<SaveError message={saveError} />
					<EventChecklist trigger={trigger} onUpdate={onUpdate} />
					<TextPatternEditor form={form} onSubmit={close} />
					<SwitchRow
						id={threadId}
						label="Don't start chats from thread replies"
					>
						<Switch
							id={threadId}
							size="sm"
							checked={Boolean(trigger.ignore_thread_replies)}
							onCheckedChange={(ignore) =>
								onUpdate({ ignore_thread_replies: ignore })
							}
						/>
					</SwitchRow>
				</>
			)}
		</SentenceChip>
	);
};

/** "Anyone", "octocat", or "3 people". */
const sendersLabel = (senders: readonly string[]): string => {
	if (senders.length === 0) {
		return "Anyone";
	}
	return senders.length === 1 ? senders[0] : `${senders.length} people`;
};

/** Limits a trigger to GitHub logins or Slack user IDs. */
export const SendersChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => {
	const isSlack = trigger.source === "slack";
	return (
		<SentenceChip
			label={sendersLabel(trigger.senders ?? [])}
			fieldLabel={isSlack ? "From" : "By"}
			readOnly={readOnly}
		>
			{() => (
				<>
					<TagInput
						label={isSlack ? "Slack user IDs" : "GitHub usernames"}
						values={[...(trigger.senders ?? [])]}
						onChange={(senders) =>
							onUpdate({
								senders: senders.map((s) => s.trim()).filter(Boolean),
							})
						}
					/>
					<p className="m-0 text-xs text-content-secondary">
						{isSlack
							? "Only these Slack user IDs, such as U0123456789, start chats. Leave empty for anyone."
							: "Only events from these GitHub usernames start chats. Leave empty for anyone."}
					</p>
				</>
			)}
		</SentenceChip>
	);
};

/** Limits a GitHub trigger to one repository or a Slack trigger to one channel. */
export const ScopeChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => {
	const inputId = useId();
	const hintId = useId();
	const isSlack = trigger.source === "slack";
	const saved = {
		scope: (isSlack ? trigger.channel : trigger.repository) ?? "",
	};
	const { form, saveError, chipProps } = useTriggerChipForm({
		saved,
		validate: ({ scope }) => {
			const error = validateTriggerScope(trigger.source, scope.trim());
			return error ? { scope: error } : {};
		},
		toRequest: ({ scope }) => {
			const next = scope.trim();
			if (next === saved.scope) {
				return {};
			}
			return isSlack ? { channel: next } : { repository: next };
		},
		onUpdate,
	});
	const error = validateTriggerScope(trigger.source, form.values.scope.trim());
	return (
		<SentenceChip
			label={saved.scope || (isSlack ? "Any channel" : "Any repository")}
			fieldLabel={isSlack ? "Channel" : "Repository"}
			readOnly={readOnly}
			{...chipProps}
		>
			{(close) => (
				<>
					<SaveError message={saveError} />
					<Label htmlFor={inputId}>
						{isSlack ? "Channel ID" : "Repository"}
					</Label>
					<Input
						id={inputId}
						name="scope"
						value={form.values.scope}
						placeholder={isSlack ? "C0123456789" : "owner/name"}
						spellCheck={false}
						autoComplete="off"
						aria-invalid={Boolean(error)}
						aria-describedby={hintId}
						className="h-8"
						onChange={form.handleChange}
						onKeyDown={(event) => {
							if (event.key === "Enter" && !error) {
								event.preventDefault();
								close();
							}
						}}
					/>
					<FieldHint id={hintId} error={error}>
						{isSlack
							? "Leave empty for every channel. The ID is at the bottom of the channel details in Slack."
							: "Leave empty for every repository that sends to this trigger."}
					</FieldHint>
				</>
			)}
		</SentenceChip>
	);
};

/** A trigger's schedule, with its time zone when it is not the viewer's. */
const scheduleLabel = (schedule: string, viewerTimeZone: string): string => {
	const description = describeSchedule(schedule);
	const timeZone = extractTimezone(schedule, "UTC");
	return timeZone === viewerTimeZone
		? description
		: `${description} (${timeZone})`;
};

type ScheduleChipProps = TriggerChipProps & {
	/** The time the editor's next-run preview counts from. */
	readonly now: Date;
	/** The viewer's time zone. */
	readonly timeZone: string;
};

/** When a schedule trigger runs: "[Every weekday at 9:00 AM ▾]". */
export const ScheduleChip: React.FC<ScheduleChipProps> = ({
	trigger,
	now,
	timeZone,
	readOnly,
	onUpdate,
}) => {
	const saved = draftFromSchedule(trigger.schedule, timeZone);
	const { form, saveError, chipProps } = useTriggerChipForm({
		saved,
		validate: (draft) => validateScheduleDraft(draft, now),
		toRequest: (draft) => {
			const schedule = scheduleFromDraft(draft);
			return schedule === scheduleFromDraft(saved) ? {} : { schedule };
		},
		onUpdate,
	});
	return (
		<SentenceChip
			label={scheduleLabel(trigger.schedule ?? "", timeZone)}
			fieldLabel="Schedule"
			readOnly={readOnly}
			contentClassName="w-80"
			{...chipProps}
		>
			{(close) => (
				<>
					<SaveError message={saveError} />
					<TriggerScheduleFields
						draft={form.values}
						errors={validateScheduleDraft(form.values, now)}
						now={now}
						timeZone={timeZone}
						onChange={(draft) => void form.setValues(draft)}
						onSubmit={close}
					/>
				</>
			)}
		</SentenceChip>
	);
};

/** A chip whose wording names the agent. */
type AgentChipProps = TriggerChipProps & {
	readonly agentName: string;
};

/**
 * Whether later events of a conversation, such as replies in a Slack
 * thread, go to the chat the conversation started.
 */
export const ConversationChip: React.FC<TriggerChipProps> = ({
	trigger,
	readOnly,
	onUpdate,
}) => {
	const switchId = useId();
	const hintId = useId();
	const source = triggerSources[trigger.source];
	return (
		<SentenceChip
			label={
				trigger.continue_conversations
					? "continue the same chat"
					: "don't continue a chat"
			}
			fieldLabel={source.followUps}
			readOnly={readOnly}
		>
			{() => (
				<>
					<SwitchRow id={switchId} label="Continue the same chat">
						<Switch
							id={switchId}
							size="sm"
							aria-describedby={hintId}
							checked={trigger.continue_conversations}
							onCheckedChange={(next) =>
								onUpdate({ continue_conversations: next })
							}
						/>
					</SwitchRow>
					<FieldHint id={hintId}>
						{source.conversationHint} An archived chat is replaced by a new one.
					</FieldHint>
				</>
			)}
		</SentenceChip>
	);
};

const busyBehaviors: Record<
	ChatProjectTriggerBusyBehavior,
	{
		/** Ends "If the agent is busy, ...". */
		readonly label: string;
		readonly optionLabel: string;
		readonly description: string;
	}
> = {
	queue: {
		label: "wait for it to finish",
		optionLabel: "Wait for it to finish",
		description: "The agent reads the new message when it is done.",
	},
	interrupt: {
		label: "interrupt it",
		optionLabel: "Interrupt it",
		description: "The agent stops and reads the new message right away.",
	},
};

/** The default, queue, comes first. */
const busyBehaviorOrder: readonly ChatProjectTriggerBusyBehavior[] = [
	"queue",
	"interrupt",
];

/** What a continued chat does with a new event while the agent works. */
export const BusyChip: React.FC<AgentChipProps> = ({
	trigger,
	agentName,
	readOnly,
	onUpdate,
}) => {
	const id = useId();
	const fieldLabel = `If ${agentName} is busy`;
	return (
		<SentenceChip
			label={busyBehaviors[trigger.busy_behavior].label}
			fieldLabel={fieldLabel}
			readOnly={readOnly}
		>
			{() => (
				<RadioGroup
					aria-label={fieldLabel}
					value={trigger.busy_behavior}
					className="gap-3"
					onValueChange={(value) => {
						const next = busyBehaviorOrder.find(
							(behavior) => behavior === value,
						);
						if (next) {
							onUpdate({ busy_behavior: next });
						}
					}}
				>
					{busyBehaviorOrder.map((behavior) => (
						<div key={behavior} className="flex items-start gap-2">
							<RadioGroupItem
								id={`${id}-${behavior}`}
								value={behavior}
								aria-describedby={`${id}-${behavior}-hint`}
								className="mt-0.5"
							/>
							<div className="flex flex-col gap-0.5">
								<Label htmlFor={`${id}-${behavior}`} className="font-normal">
									{busyBehaviors[behavior].optionLabel}
								</Label>
								<p
									id={`${id}-${behavior}-hint`}
									className="m-0 text-xs text-content-secondary"
								>
									{busyBehaviors[behavior].description}
								</p>
							</div>
						</div>
					))}
				</RadioGroup>
			)}
		</SentenceChip>
	);
};

type TriggerSecretChipProps = TriggerChipProps & {
	readonly secret: TriggerSecretInfo;
};

/** Whether Coder checks a Slack or GitHub trigger's requests with a secret. */
export const TriggerSecretChip: React.FC<TriggerSecretChipProps> = ({
	trigger,
	secret,
	readOnly,
	onUpdate,
}) => (
	<SentenceChip
		label={secret.isSet(trigger) ? "Set" : "Not set"}
		fieldLabel={secret.title}
		readOnly={readOnly}
		contentClassName="w-80"
	>
		{() => (
			<TriggerSecretFields
				trigger={trigger}
				secret={secret}
				onUpdate={onUpdate}
			/>
		)}
	</SentenceChip>
);

/** What the trigger asks the agent to do, and the trigger's name. */
export const PromptChip: React.FC<AgentChipProps> = ({
	trigger,
	agentName,
	readOnly,
	onUpdate,
}) => {
	const nameId = useId();
	const promptId = useId();
	const { form, saveError, chipProps } = useTriggerChipForm({
		saved: { name: trigger.name, prompt: trigger.prompt },
		toRequest: ({ name, prompt }) => {
			const nextName = name.trim() || trigger.name;
			const nextPrompt = prompt.trim();
			return nextName === trigger.name && nextPrompt === trigger.prompt
				? {}
				: { name: nextName, prompt: nextPrompt };
		},
		onUpdate,
	});
	return (
		<SentenceChip
			label={trigger.prompt || "Add prompt"}
			fieldLabel={`Ask ${agentName} to`}
			readOnly={readOnly}
			contentClassName="w-96"
			{...chipProps}
		>
			{(close) => (
				<>
					<SaveError message={saveError} />
					<Label htmlFor={promptId}>Prompt</Label>
					<Textarea
						id={promptId}
						name="prompt"
						rows={5}
						value={form.values.prompt}
						maxLength={triggerPromptMaxLength}
						placeholder={triggerSources[trigger.source].promptPlaceholder}
						onChange={form.handleChange}
					/>
					<Label htmlFor={nameId}>Name</Label>
					<Input
						id={nameId}
						name="name"
						value={form.values.name}
						maxLength={triggerNameMaxLength}
						className="h-8"
						onChange={form.handleChange}
					/>
					<p className="m-0 text-xs text-content-secondary">
						Also the title of each chat this trigger starts.
					</p>
					<div className="flex justify-end">
						<Button size="sm" onClick={close}>
							Done
						</Button>
					</div>
				</>
			)}
		</SentenceChip>
	);
};

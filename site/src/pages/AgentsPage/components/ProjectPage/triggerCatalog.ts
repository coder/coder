import {
	type ChatProjectTrigger,
	ChatProjectTriggerConversationHeader,
	ChatProjectTriggerSlackEventAppMention,
	ChatProjectTriggerSlackEventMessage,
	ChatProjectTriggerSlackEventSlashCommand,
	type ChatProjectTriggerSource,
	type UpdateChatProjectTriggerRequest,
} from "#/api/typesGenerated";
import { describeSchedule } from "./triggerSchedule";

/** A trigger event a person can pick. */
type TriggerEventOption = {
	readonly value: string;
	/** Shown on the event chip, such as "App mention". */
	readonly label: string;
	/** Shown in the add menu and event checklists, such as "App mentions". */
	readonly optionLabel: string;
};

type TriggerSourceInfo = {
	readonly title: string;
	/** Undefined for webhook and schedule triggers, which use lucide icons. */
	readonly iconURL?: string;
	readonly events: readonly TriggerEventOption[];
	/** Chip label for a trigger that listens to every event. */
	readonly anyEventLabel: string;
	/**
	 * The later events of one conversation, such as "Replies in the
	 * thread". Plural, to start a sentence like "... continue the same chat".
	 */
	readonly followUps: string;
	/** Which events count as one conversation. */
	readonly conversationHint: string;
} & TriggerRunWording;

/** How a source's runs are worded around the trigger. */
type TriggerRunWording = {
	/** Starts the menu note "Last triggered 2 hours ago". */
	readonly lastRunPrefix: string;
	/** The menu note before the first run. */
	readonly neverRan: string;
	/** Titles the list of recent webhook calls or runs. */
	readonly historyTitle: string;
	/** Shown while that list is empty. */
	readonly historyEmpty: string;
	/** What deleting the trigger stops, for the delete confirmation. */
	readonly deleteInfo: string;
	/** Delivery events in words, such as "Run now". Others show as code. */
	readonly deliveryEventLabels?: Partial<Record<string, string>>;
	readonly promptPlaceholder: string;
};

const webhookWording: TriggerRunWording = {
	lastRunPrefix: "Last triggered",
	neverRan: "Never triggered",
	historyTitle: "Recent deliveries",
	historyEmpty: "No deliveries yet.",
	deleteInfo:
		"Events sent to this trigger's webhook URL will stop starting chats.",
	promptPlaceholder:
		"Review the pull request and leave a comment with your findings.",
};

const githubEvent = (value: string, label: string): TriggerEventOption => ({
	value,
	label,
	optionLabel: label,
});

/** Display names and event catalogs for each trigger source. */
export const triggerSources: Record<
	ChatProjectTriggerSource,
	TriggerSourceInfo
> = {
	github: {
		...webhookWording,
		title: "GitHub",
		iconURL: "/icon/github.svg",
		anyEventLabel: "Any event",
		followUps: "New comments and updates on the same issue or pull request",
		conversationHint:
			"Later events this trigger listens to on the same issue or pull request go to the chat the first one started.",
		events: [
			githubEvent("pull_request.opened", "Pull request opened"),
			githubEvent("pull_request.closed", "Pull request closed"),
			githubEvent("pull_request.synchronize", "Pull request updated"),
			githubEvent("pull_request_review.submitted", "Review submitted"),
			githubEvent("issues.opened", "Issue opened"),
			githubEvent("issues.labeled", "Issue labeled"),
			githubEvent("issue_comment.created", "New comment"),
			githubEvent("push", "Push"),
			githubEvent("release.published", "Release published"),
			githubEvent("workflow_run.completed", "Workflow run completed"),
		],
	},
	slack: {
		...webhookWording,
		title: "Slack",
		iconURL: "/icon/slack.svg",
		anyEventLabel: "Any Slack event",
		followUps: "Replies in the thread",
		conversationHint:
			"Replies in a thread go to the chat that thread started, even when they don't match the text filter.",
		events: [
			{
				value: ChatProjectTriggerSlackEventMessage,
				label: "Any message",
				optionLabel: "Messages",
			},
			{
				value: ChatProjectTriggerSlackEventAppMention,
				label: "App mention",
				optionLabel: "App mentions",
			},
			{
				value: ChatProjectTriggerSlackEventSlashCommand,
				label: "Slash command",
				optionLabel: "Slash command",
			},
		],
	},
	webhook: {
		...webhookWording,
		title: "Webhook",
		anyEventLabel: "Webhook call",
		followUps: `Calls with the same ${ChatProjectTriggerConversationHeader} header`,
		conversationHint: `Calls that send the same ${ChatProjectTriggerConversationHeader} header value go to one chat. Calls without the header always start a new chat.`,
		events: [],
	},
	schedule: {
		title: "Schedule",
		anyEventLabel: "Scheduled run",
		followUps: "Later runs",
		conversationHint:
			"Every run, including Run now, goes to the chat the first run started.",
		events: [],
		lastRunPrefix: "Last ran",
		neverRan: "Never ran",
		historyTitle: "Recent runs",
		historyEmpty: "No runs yet.",
		deleteInfo: "Its schedule will stop starting chats.",
		deliveryEventLabels: { schedule: "Scheduled", run_now: "Run now" },
		promptPlaceholder: "Summarize the pull requests merged since the last run.",
	},
};

/** Events outside the catalog show their raw name, such as `deployment`. */
const triggerEventOption = (
	source: ChatProjectTriggerSource,
	value: string,
): TriggerEventOption =>
	triggerSources[source].events.find((option) => option.value === value) ?? {
		value,
		label: value,
		optionLabel: value,
	};

/** Catalog events followed by any other events the trigger already has. */
export const triggerEventOptions = (
	source: ChatProjectTriggerSource,
	events: readonly string[],
): readonly TriggerEventOption[] => {
	const catalog = triggerSources[source].events;
	const extra = events
		.filter((event) => !catalog.some((option) => option.value === event))
		.map((event) => triggerEventOption(source, event));
	return [...catalog, ...extra];
};

/** "Any event", "Issue opened", "Issue opened or Push", "Push and 2 more". */
export const triggerEventsLabel = (
	source: ChatProjectTriggerSource,
	events: readonly string[],
): string => {
	const labels = events.map((event) => triggerEventOption(source, event).label);
	if (labels.length === 0) {
		return triggerSources[source].anyEventLabel;
	}
	if (labels.length <= 2) {
		return labels.join(" or ");
	}
	return `${labels[0]} and ${labels.length - 1} more`;
};

/** Keep in sync with chatProjectTriggerNameMaxChars in coderd/chat_project_agents.go. */
export const triggerNameMaxLength = 64;

/** Keep in sync with chatProjectTriggerPromptMaxChars in coderd/chat_project_agents.go. */
export const triggerPromptMaxLength = 8192;

/**
 * Keep in sync with normalizeChatProjectTriggerConversationOptions and
 * normalizeChatProjectTriggerGitHubWebhookSecret in
 * coderd/chat_project_agents.go.
 */
export const triggerSecretMaxLength = 256;

/** The secret a Slack or GitHub trigger checks requests with. */
export type TriggerSecretInfo = {
	/** Labels the field and the chip, such as "Signing secret". */
	readonly label: string;
	/** Starts the trigger's secret line, such as "Slack signing secret". */
	readonly title: string;
	/** Where the secret comes from and what setting it does. */
	readonly hint: string;
	/** What Coder does while a secret is set. */
	readonly verifiedHint: string;
	readonly isSet: (trigger: ChatProjectTrigger) => boolean;
	/** Saves `value`, or removes the secret when it is empty. */
	readonly toRequest: (value: string) => UpdateChatProjectTriggerRequest;
};

/** The secret of a Slack or GitHub trigger. Plain webhooks have none. */
export const triggerSecrets: Partial<
	Record<ChatProjectTriggerSource, TriggerSecretInfo>
> = {
	slack: {
		label: "Signing secret",
		title: "Slack signing secret",
		hint: "Optional. In your Slack app, open Basic Information and copy the Signing Secret. Coder then checks that every request comes from Slack.",
		verifiedHint: "Coder checks that every request comes from your Slack app.",
		isSet: (trigger) => trigger.has_slack_signing_secret,
		toRequest: (value) => ({ slack_signing_secret: value }),
	},
	github: {
		label: "Webhook secret",
		title: "GitHub webhook secret",
		hint: "Optional. Choose a long random value and set the same secret in the GitHub webhook's Secret field. Coder then checks that every delivery comes from GitHub.",
		verifiedHint:
			"Coder checks that every delivery is signed with the GitHub webhook's secret.",
		isSet: (trigger) => trigger.has_github_webhook_secret,
		toRequest: (value) => ({ github_webhook_secret: value }),
	},
};

/**
 * Whether anyone with the webhook URL could get past the trigger's sender,
 * channel, or repository filters, because no secret proves where a request
 * came from.
 */
export const hasUnverifiedFilters = (trigger: ChatProjectTrigger): boolean => {
	const secret = triggerSecrets[trigger.source];
	if (!secret || secret.isSet(trigger)) {
		return false;
	}
	return (
		(trigger.senders?.length ?? 0) > 0 ||
		Boolean(trigger.channel) ||
		Boolean(trigger.repository)
	);
};

/**
 * A name that describes the trigger, such as "Pull request opened in
 * coder/coder", cut to the name limit.
 */
export const suggestTriggerName = (
	trigger: Pick<
		ChatProjectTrigger,
		"source" | "events" | "repository" | "channel" | "schedule"
	>,
): string => {
	if (trigger.source === "webhook") {
		return "Webhook";
	}
	if (trigger.source === "schedule") {
		return Array.from(describeSchedule(trigger.schedule ?? ""))
			.slice(0, triggerNameMaxLength)
			.join("");
	}
	const scope = trigger.repository || trigger.channel;
	const name = triggerEventsLabel(trigger.source, trigger.events);
	// The server counts characters, not UTF-16 code units.
	return Array.from(scope ? `${name} in ${scope}` : name)
		.slice(0, triggerNameMaxLength)
		.join("");
};

/**
 * Renames a trigger along with its sentence while it still has the name
 * suggested for it, so names never set by hand stay accurate.
 */
export const withSuggestedName = (
	trigger: ChatProjectTrigger,
	request: UpdateChatProjectTriggerRequest,
): UpdateChatProjectTriggerRequest => {
	if (
		request.name !== undefined ||
		trigger.name !== suggestTriggerName(trigger)
	) {
		return request;
	}
	const name = suggestTriggerName({ ...trigger, ...request });
	return name === trigger.name ? request : { ...request, name };
};

const repositoryPattern = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
const channelPattern = /^[A-Z0-9]+$/;

/** Mirrors the server's checks so mistakes show before saving. */
export const validateTriggerScope = (
	source: ChatProjectTriggerSource,
	scope: string,
): string | undefined => {
	if (!scope) {
		return undefined;
	}
	if (source === "github" && !repositoryPattern.test(scope)) {
		return "Use the form owner/name, such as coder/coder.";
	}
	if (source === "slack" && !channelPattern.test(scope)) {
		return scope.startsWith("#")
			? "Use the channel ID, such as C0123456789, not the channel name."
			: "A channel ID has only capital letters and digits, such as C0123456789.";
	}
	return undefined;
};

/**
 * Catches most broken regexes before saving. JavaScript and the server's RE2
 * syntax differ, so this accepts RE2 flag groups and named groups, and the
 * server still has the final say.
 */
export const validateTriggerTextPattern = (
	pattern: string,
	isRegex: boolean,
): string | undefined => {
	if (!isRegex || !pattern) {
		return undefined;
	}
	const javascriptPattern = pattern
		.replace(/\(\?[imsU-]+\)/g, "")
		.replace(/\(\?[imsU-]+:/g, "(?:")
		.replace(/\(\?P</g, "(?<");
	try {
		new RegExp(javascriptPattern);
		return undefined;
	} catch {
		return "This is not a valid regular expression.";
	}
};

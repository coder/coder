import { Fragment } from "react";
import {
	type ChatProjectTrigger,
	ChatProjectTriggerConversationHeader,
	ChatProjectTriggerSlackEventAppMention,
	ChatProjectTriggerSlackEventMessage,
	ChatProjectTriggerSlackEventSlashCommand,
} from "#/api/typesGenerated";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { TriggerSecretFields } from "./TriggerChipEditors";
import type { TriggerChipProps } from "./TriggerSentenceChip";
import { triggerSecrets } from "./triggerCatalog";

type TriggerSetupStepsProps = {
	readonly trigger: ChatProjectTrigger;
	/** Embeds the secret, for the example request. */
	readonly webhookURL: string;
	readonly onUpdate: TriggerChipProps["onUpdate"];
};

/** Tells the person where to paste the webhook URL for the trigger's source. */
export const TriggerSetupSteps: React.FC<TriggerSetupStepsProps> = ({
	trigger,
	webhookURL,
	onUpdate,
}) => {
	switch (trigger.source) {
		case "github":
			return <GitHubSetupSteps trigger={trigger} onUpdate={onUpdate} />;
		case "slack":
			return <SlackSetupSteps trigger={trigger} onUpdate={onUpdate} />;
		default:
			return <WebhookSetupSteps trigger={trigger} webhookURL={webhookURL} />;
	}
};

type WebhookSetupStepsProps = {
	readonly trigger: ChatProjectTrigger;
	readonly webhookURL: string;
};

const WebhookSetupSteps: React.FC<WebhookSetupStepsProps> = ({
	trigger,
	webhookURL,
}) => {
	// The URL comes from the server and has no quotes to escape.
	const example = [
		`curl -X POST '${webhookURL}'`,
		`  -H 'Content-Type: application/json'`,
		`  -H '${ChatProjectTriggerConversationHeader}: ticket-123'`,
		`  -d '{"text": "Customer replied to ticket 123"}'`,
	].join(" \\\n");
	return (
		<SetupStepList title="Call the webhook">
			<li>
				Send a <Code>POST</Code> request with a JSON body to the URL from your
				service. The agent gets the request body as untrusted input.
			</li>
			<li>
				To keep related calls in one chat, send the same{" "}
				<Code>{ChatProjectTriggerConversationHeader}</Code> header value with
				each of them
				{trigger.continue_conversations ? (
					". Calls without it start a new chat."
				) : (
					<>
						, and turn on <strong>Continue the same chat</strong> for this
						trigger. Until then, each call starts a new chat.
					</>
				)}
				<div className="mt-2 flex items-start gap-2 rounded-md border border-solid border-border bg-surface-primary p-2">
					<pre className="m-0 min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-xs text-content-primary">
						{example}
					</pre>
					<CopyButton text={example} label="Copy example request" />
				</div>
			</li>
		</SetupStepList>
	);
};

type SourceSetupStepsProps = {
	readonly trigger: ChatProjectTrigger;
	readonly onUpdate: TriggerChipProps["onUpdate"];
};

/** The optional secret step's editor, for sources that have a secret. */
const SecretStepFields: React.FC<SourceSetupStepsProps> = ({
	trigger,
	onUpdate,
}) => {
	const secret = triggerSecrets[trigger.source];
	if (!secret) {
		return null;
	}
	return (
		<div className="mt-2 max-w-sm">
			<TriggerSecretFields
				trigger={trigger}
				secret={secret}
				onUpdate={onUpdate}
			/>
		</div>
	);
};

const GitHubSetupSteps: React.FC<SourceSetupStepsProps> = ({
	trigger,
	onUpdate,
}) => {
	const webhookEvents = githubWebhookEvents(trigger.events);
	return (
		<SetupStepList title="Connect GitHub">
			<li>
				In the repository or organization, open{" "}
				<strong>Settings &gt; Webhooks</strong> and select{" "}
				<strong>Add webhook</strong>.
			</li>
			<li>
				Paste the URL into <strong>Payload URL</strong>.
			</li>
			<li>
				Set <strong>Content type</strong> to <Code>application/json</Code>.
			</li>
			<li>
				Optionally, set the same secret in the GitHub webhook's{" "}
				<strong>Secret</strong> field and save it here, so Coder only accepts
				deliveries signed by GitHub.
				<SecretStepFields trigger={trigger} onUpdate={onUpdate} />
			</li>
			<li>
				{webhookEvents.length === 0 ? (
					<>
						Under events, choose <strong>Send me everything</strong>.
					</>
				) : (
					<>
						Under events, choose{" "}
						<strong>Let me select individual events</strong> and check{" "}
						<JoinWithAnd items={webhookEvents} format="strong" />.
					</>
				)}
			</li>
			<li>
				Select <strong>Add webhook</strong>. GitHub sends a test ping, which
				Coder answers without starting a chat.
			</li>
		</SetupStepList>
	);
};

/** Slack sends messages in channels, private channels, and DMs as separate bot events. */
const slackMessageBotEvents = [
	"message.channels",
	"message.groups",
	"message.im",
];

const SlackSetupSteps: React.FC<SourceSetupStepsProps> = ({
	trigger,
	onUpdate,
}) => {
	const { events } = trigger;
	const listensTo = (event: string) =>
		events.length === 0 || events.includes(event);
	const listensToMentions = listensTo(ChatProjectTriggerSlackEventAppMention);
	// Thread replies arrive as message events, even when the trigger only
	// starts chats for mentions.
	const repliesContinue = trigger.continue_conversations && listensToMentions;
	const needsMessages =
		listensTo(ChatProjectTriggerSlackEventMessage) || repliesContinue;
	const botEvents = [
		...(listensToMentions ? [ChatProjectTriggerSlackEventAppMention] : []),
		...(needsMessages ? slackMessageBotEvents : []),
	];
	return (
		<SetupStepList title="Connect Slack">
			{botEvents.length > 0 && (
				<>
					<li>
						Open your app at <strong>api.slack.com/apps</strong>, go to{" "}
						<strong>Event Subscriptions</strong>, and turn on{" "}
						<strong>Enable Events</strong>.
					</li>
					<li>
						Paste the URL into <strong>Request URL</strong>. Coder answers
						Slack's verification for you.
					</li>
					<li>
						Under <strong>Subscribe to bot events</strong>, add{" "}
						<JoinWithAnd items={botEvents} format="code" />, then save.
						{repliesContinue &&
							" The message events bring the thread replies that continue a chat."}
					</li>
				</>
			)}
			{listensTo(ChatProjectTriggerSlackEventSlashCommand) && (
				<li>
					For slash commands, open <strong>Slash Commands</strong>, create or
					edit a command, and paste the URL into <strong>Request URL</strong>.
				</li>
			)}
			<li>
				Reinstall the app if Slack asks, and invite it to the channels it should
				hear. Messages from bots are ignored.
			</li>
			<li>
				Optionally, add the app's signing secret so Coder only accepts requests
				signed by Slack.
				<SecretStepFields trigger={trigger} onUpdate={onUpdate} />
			</li>
		</SetupStepList>
	);
};

type SetupStepListProps = {
	readonly title: string;
	readonly children: React.ReactNode;
};

const SetupStepList: React.FC<SetupStepListProps> = ({ title, children }) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-medium text-content-primary">{title}</h3>
		<ol className="m-0 flex list-decimal flex-col gap-1.5 pl-5 text-sm text-content-secondary [&_strong]:font-medium [&_strong]:text-content-primary">
			{children}
		</ol>
	</div>
);

type CodeProps = {
	readonly children: React.ReactNode;
};

const Code: React.FC<CodeProps> = ({ children }) => (
	<code className="rounded bg-surface-secondary px-1 py-0.5 font-mono text-xs text-content-primary">
		{children}
	</code>
);

type JoinWithAndProps = {
	readonly items: readonly string[];
	readonly format: "strong" | "code";
};

/** Renders "a", "a and b", or "a, b and c". */
const JoinWithAnd: React.FC<JoinWithAndProps> = ({ items, format }) =>
	items.map((item, index) => (
		<Fragment key={item}>
			{index > 0 && (index === items.length - 1 ? " and " : ", ")}
			{format === "code" ? <Code>{item}</Code> : <strong>{item}</strong>}
		</Fragment>
	));

/** GitHub's names for the event checkboxes in its webhook settings. */
const githubWebhookEventNames: Record<string, string> = {
	pull_request: "Pull requests",
	pull_request_review: "Pull request reviews",
	issues: "Issues",
	issue_comment: "Issue comments",
	push: "Pushes",
	release: "Releases",
	workflow_run: "Workflow runs",
};

/** The GitHub webhook event checkboxes needed for the trigger's events. */
const githubWebhookEvents = (events: readonly string[]): string[] => {
	const names = new Set<string>();
	for (const event of events) {
		const header = event.split(".")[0];
		names.add(githubWebhookEventNames[header] ?? header);
	}
	return [...names];
};

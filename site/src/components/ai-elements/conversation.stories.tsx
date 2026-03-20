import type { Meta, StoryObj } from "@storybook/react-vite";
import { Conversation, ConversationItem } from "./conversation";
import { Message, MessageContent } from "./message";
import { Shimmer } from "./shimmer";
import { Thinking } from "./thinking";

const meta: Meta<typeof Conversation> = {
	title: "components/ai-elements/Conversation",
	component: Conversation,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof Conversation>;

export const ConversationWithMessages: Story = {
	render: () => {
		const userItemProps = { role: "user" as const };
		const assistantItemProps = { role: "assistant" as const };

		return (
			<Conversation>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							Check why `git fetch` is failing in this workspace.
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<Thinking>
									Inspecting auth state and recent command output before
									suggesting a fix.
								</Thinking>
								<div className="text-sm text-content-primary">
									The remote command failed because external auth needs to be
									refreshed.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
			</Conversation>
		);
	},
};

export const LoadingState: Story = {
	render: () => {
		const assistantItemProps = { role: "assistant" as const };

		return (
			<Conversation>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<Shimmer as="span" className="text-sm">
								Thinking...
							</Shimmer>
						</MessageContent>
					</Message>
				</ConversationItem>
			</Conversation>
		);
	},
};

export const MultipleConsecutiveMessages: Story = {
	render: () => {
		const userItemProps = { role: "user" as const };
		const assistantItemProps = { role: "assistant" as const };

		return (
			<Conversation>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							Why is the API returning 500 errors?
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<div className="text-sm text-content-primary">
									Let me check the server logs. It looks like the database
									connection pool is exhausted.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							How do I increase the pool size?
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<div className="text-sm text-content-primary">
									You can update the <code>DB_MAX_CONNECTIONS</code>
									environment variable in your deployment config. The default is
									25 connections.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							Done. Should I restart the service?
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<div className="text-sm text-content-primary">
									Yes, restart the coderd service for the change to take effect.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
			</Conversation>
		);
	},
};

export const MixedContentTypes: Story = {
	render: () => {
		const userItemProps = { role: "user" as const };
		const assistantItemProps = { role: "assistant" as const };

		return (
			<Conversation>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							Can you help diagnose this deployment issue?
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<Thinking>
									Reviewing the latest rollout events and recent health checks
									before suggesting the next step.
								</Thinking>
								<div className="text-sm text-content-primary">
									The new replica started, but it never passed readiness. The
									probe is timing out before the app finishes booting.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-full max-w-none">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							The app does a schema check on startup and warms the cache before
							serving traffic. Should I raise the readiness timeout, or is there
							a better way to confirm the boot sequence is actually healthy?
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<div className="text-sm text-content-primary">
									Start by increasing the readiness timeout so the probe matches
									the real startup path. That prevents the rollout controller
									from recycling pods that are still initializing.
								</div>
								<div className="text-sm text-content-primary">
									After that, add a lightweight health endpoint that checks
									dependencies without running the full warm-up routine. That
									gives you a faster signal that the process is alive while you
									keep readiness focused on serving traffic safely.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<Shimmer as="span" className="text-sm">
								Drafting rollout recommendations...
							</Shimmer>
						</MessageContent>
					</Message>
				</ConversationItem>
			</Conversation>
		);
	},
};

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

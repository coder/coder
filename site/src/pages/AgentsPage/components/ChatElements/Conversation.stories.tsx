import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConversationItem } from "./Conversation";
import { Message, MessageContent } from "./Message";
import { Shimmer } from "./Shimmer";

const meta: Meta<typeof ConversationItem> = {
	title: "pages/AgentsPage/ChatElements/Conversation",
	component: ConversationItem,
};

export default meta;
type Story = StoryObj<typeof ConversationItem>;

export const ConversationWithMessages: Story = {
	render: () => {
		const userItemProps = { role: "user" as const };
		const assistantItemProps = { role: "assistant" as const };

		return (
			<div className="flex flex-col gap-5">
				<ConversationItem {...userItemProps}>
					<Message className="my-2 w-fit max-w-[min(80vw,80%)]">
						<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
							Check why `git fetch` is failing in this workspace.
						</MessageContent>
					</Message>
				</ConversationItem>
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<div className="space-y-3">
								<div className="text-sm text-content-primary">
									The remote command failed because external auth needs to be
									refreshed.
								</div>
							</div>
						</MessageContent>
					</Message>
				</ConversationItem>
			</div>
		);
	},
};

export const LoadingState: Story = {
	render: () => {
		const assistantItemProps = { role: "assistant" as const };

		return (
			<div className="flex flex-col gap-5">
				<ConversationItem {...assistantItemProps}>
					<Message className="w-full">
						<MessageContent className="whitespace-normal">
							<Shimmer as="span" className="text-sm">
								Thinking...
							</Shimmer>
						</MessageContent>
					</Message>
				</ConversationItem>
			</div>
		);
	},
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockChatProjectInstructions,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";

const meta: Meta<typeof ProjectDetailsPanelView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectDetailsPanelView",
	component: ProjectDetailsPanelView,
	decorators: [
		(Story) => (
			<div className="max-w-[30rem]">
				<Story />
			</div>
		),
	],
	args: {
		instructions: MockChatProjectInstructions,
		error: undefined,
		onRetry: fn(),
		onEditInstructions: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectDetailsPanelView>;

export const Unset: Story = {
	args: { instructions: MockUnsetChatProjectInstructions },
};

/** Long instructions are clamped to three lines in the preview. */
export const LongInstructions: Story = {
	args: {
		instructions: {
			...MockChatProjectInstructions,
			instructions: Array.from(
				{ length: 8 },
				(_, index) =>
					`${index + 1}. Follow the repository conventions for this kind of change and explain the tradeoffs in the pull request description.`,
			).join("\n"),
		},
	},
};

export const Loading: Story = {
	args: { instructions: undefined },
};

export const LoadError: Story = {
	args: {
		instructions: undefined,
		error: mockApiError({
			message: "Failed to get chat project instructions.",
		}),
	},
};

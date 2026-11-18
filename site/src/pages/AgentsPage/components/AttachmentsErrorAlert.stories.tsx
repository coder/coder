import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AttachmentsErrorAlert } from "./AttachmentsErrorAlert";

const meta: Meta<typeof AttachmentsErrorAlert> = {
	title: "pages/AgentsPage/AttachmentsErrorAlert",
	component: AttachmentsErrorAlert,
	args: {
		isRetrying: false,
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AttachmentsErrorAlert>;

const overviewCardWidth = (Story: React.FC) => (
	<div className="w-64 p-1.5">
		<Story />
	</div>
);

export const OverviewCard: Story = {
	decorators: [overviewCardWidth],
};

export const OverviewCardRetrying: Story = {
	args: { isRetrying: true },
	decorators: [overviewCardWidth],
};

export const AttachmentsList: Story = {
	decorators: [
		(Story) => (
			<div className="w-90 p-4">
				<Story />
			</div>
		),
	],
};

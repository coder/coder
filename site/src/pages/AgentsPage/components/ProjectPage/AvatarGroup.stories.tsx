import type { Meta, StoryObj } from "@storybook/react-vite";
import { AvatarGroup } from "./AvatarGroup";

const mockItems = ["Ada", "Grace", "Linus", "Margaret", "Dennis"].map(
	(name) => ({ id: name, name }),
);

const meta: Meta<typeof AvatarGroup> = {
	title: "pages/AgentsPage/ProjectPage/AvatarGroup",
	component: AvatarGroup,
	args: { items: mockItems, label: "Shared with 5 users and groups" },
};

export default meta;
type Story = StoryObj<typeof AvatarGroup>;

export const WithOverflow: Story = {};

export const WithoutOverflow: Story = {
	args: {
		items: mockItems.slice(0, 2),
		label: "Shared with 2 users and groups",
	},
};

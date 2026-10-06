import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatProjectIcon } from "./ChatProjectIcon";

const meta: Meta<typeof ChatProjectIcon> = {
	title: "pages/AgentsPage/ChatProjectIcon",
	component: ChatProjectIcon,
	args: { className: "size-8" },
};

export default meta;
type Story = StoryObj<typeof ChatProjectIcon>;

export const Image: Story = {
	args: { project: { icon: "/emojis/1f680.png" } },
};

export const Folder: Story = {
	args: { project: { icon: "" } },
};

export const OpenFolder: Story = {
	args: { project: { icon: "" }, expanded: true },
};

export const FailedImage: Story = {
	args: { project: { icon: "/emojis/does-not-exist.png" } },
};

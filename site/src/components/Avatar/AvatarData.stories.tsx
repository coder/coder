import type { Meta, StoryObj } from "@storybook/react-vite";
import { AvatarData } from "./AvatarData";

const meta: Meta<typeof AvatarData> = {
	title: "components/AvatarData",
	component: AvatarData,
	args: {
		title: "coder",
		subtitle: "coder@coder.com",
	},
};

export default meta;
type Story = StoryObj<typeof AvatarData>;

export const WithTitleAndSubtitle: Story = {};

export const WithImage: Story = {
	args: {
		src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
	},
};

export const WithLongTitle: Story = {
	args: {
		truncate: true,
		title: "a-workspace-with-an-unreasonably-long-name-that-should-be-clipped",
		subtitle:
			"and-an-even-longer-organization-or-template-subtitle-that-truncates",
	},
	decorators: [(Story) => <div style={{ maxWidth: 240 }}>{Story()}</div>],
};

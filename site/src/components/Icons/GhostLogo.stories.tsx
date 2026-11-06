import type { Meta, StoryObj } from "@storybook/react-vite";
import { GhostLogo } from "./GhostLogo";

const meta: Meta<typeof GhostLogo> = {
	title: "components/Icons/GhostLogo",
	component: GhostLogo,
	args: { className: "h-7" },
	decorators: [
		(Story) => (
			<div className="p-10">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof GhostLogo>;

/** Hover the logo to see the easter egg. */
export const Default: Story = {};

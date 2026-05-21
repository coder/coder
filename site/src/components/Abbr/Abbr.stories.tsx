import type { Meta, StoryObj } from "@storybook/react-vite";
import { Abbr } from "./Abbr";

const meta: Meta<typeof Abbr> = {
	title: "components/Abbr",
	component: Abbr,
	decorators: [
		(Story) => (
			<div className="max-w-prose text-base">
				<p>Try the following text out in a screen reader!</p>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof Abbr>;

export const InlinedShorthand: Story = {
	args: {
		pronunciation: "shorthand",
		children: "ms",
		title: "milliseconds",
	},
	decorators: [
		(Story) => (
			<p>
				The physical pain of getting bonked on the head with a cartoon mallet
				lasts precisely 593
				<span className="underline decoration-dotted">
					<Story />
				</span>
				. The emotional turmoil and complete embarrassment lasts forever.
			</p>
		),
	],
};

export const Acronym: Story = {
	args: {
		pronunciation: "acronym",
		children: "NASA",
		title: "National Aeronautics and Space Administration",
	},
	decorators: [
		(Story) => (
			<span className="underline decoration-dotted">
				<Story />
			</span>
		),
	],
};

export const Initialism: Story = {
	args: {
		pronunciation: "initialism",
		children: "CLI",
		title: "Command-Line Interface",
	},
	decorators: [
		(Story) => (
			<span className="underline decoration-dotted">
				<Story />
			</span>
		),
	],
};

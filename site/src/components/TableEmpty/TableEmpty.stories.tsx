import type { Meta, StoryObj } from "@storybook/react-vite";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Table, TableBody } from "components/Table/Table";
import { TableEmpty } from "./TableEmpty";

const meta: Meta<typeof TableEmpty> = {
	title: "components/TableEmpty",
	component: TableEmpty,
	args: {
		message: "Unfortunately, there's a radio connected to my brain",
	},
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof TableEmpty>;

export const Example: Story = {};

export const WithImageAndCta: Story = {
	name: "With Image and CTA",
	args: {
		description: "A gruff voice crackles to life on the intercom.",
		cta: (
			<CodeExample
				secret={false}
				code="say &ldquo;Actually, it's the BBC controlling us from London&rdquo;"
			/>
		),
		image: (
			<img
				src="/featured/templates.webp"
				alt=""
				className="max-w-3xl h-[320px] overflow-hidden object-cover object-top"
			/>
		),
		style: { paddingBottom: 0 },
	},
};

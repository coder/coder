import type { Meta, StoryObj } from "@storybook/react-vite";
import { Search, SearchEmpty, SearchInput } from "./Search";

const meta: Meta<typeof SearchInput> = {
	title: "components/Search",
	component: SearchInput,
};

export default meta;
type Story = StoryObj<typeof SearchInput>;

export const Example: Story = {
	render: (props) => (
		<Search>
			<SearchInput {...props} />
		</Search>
	),
};

export const WithCustomPlaceholder: Story = {
	args: {
		label: "uwu",
		placeholder: "uwu",
	},
	render: (props) => (
		<Search>
			<SearchInput {...props} />
		</Search>
	),
};

export const WithSearchEmpty: Story = {
	args: {
		label: "I crave the certainty of steel",
		placeholder: "Alas, I am empty",
	},
	render: (props) => (
		<div className="flex flex-col gap-2">
			<Search>
				<SearchInput {...props} />
			</Search>

			<SearchEmpty />
		</div>
	),
};

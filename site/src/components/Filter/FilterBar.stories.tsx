import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { UseFilterResult } from "./Filter";
import { FilterBar, FilterPresetMenu, FilterSearchField } from "./Filter";

const mockFilter: UseFilterResult = {
	query: "",
	values: {},
	used: false,
	update: fn(),
	debounceUpdate: fn(),
	cancelDebounce: fn(),
};

const meta: Meta<typeof FilterBar> = {
	title: "components/Filter/FilterBar",
	component: FilterBar,
};

export default meta;
type Story = StoryObj<typeof FilterBar>;

export const Empty: Story = {
	args: {
		children: <div className="h-9 w-40 rounded bg-surface-secondary" />,
	},
};

export const WithFilterPieces: Story = {
	args: {
		children: (
			<>
				<FilterPresetMenu
					value=""
					presets={[
						{ name: "My workspaces", query: "owner:me" },
						{ name: "All workspaces", query: "" },
					]}
					onSelect={fn()}
				/>
				<FilterSearchField filter={mockFilter} />
			</>
		),
	},
};

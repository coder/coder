import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import { ListSessionsFilter } from "./ListSessionsFilter";

type FilterProps = ComponentProps<typeof ListSessionsFilter>;

const defaultFilterProps = getDefaultFilterProps<
	Pick<FilterProps, "filter" | "menus">
>({
	query: "",
	values: {
		username: undefined,
		provider: undefined,
	},
	menus: {
		user: MockMenu,
		provider: MockMenu,
		client: MockMenu,
		model: MockMenu,
	},
});

const meta: Meta<typeof ListSessionsFilter> = {
	title: "pages/AIBridgePage/ListSessionsFilter",
	component: ListSessionsFilter,
};

export default meta;
type Story = StoryObj<typeof ListSessionsFilter>;

export const Default: Story = {
	args: {
		...defaultFilterProps,
	},
};

export const WithQuery: Story = {
	args: {
		...getDefaultFilterProps<Pick<FilterProps, "filter" | "menus">>({
			query: "initiator:me",
			values: {
				username: "me",
				provider: undefined,
			},
			menus: {
				user: MockMenu,
				provider: MockMenu,
				client: MockMenu,
				model: MockMenu,
			},
			used: true,
		}),
	},
};

export const Loading: Story = {
	args: {
		...defaultFilterProps,
		menus: {
			user: { ...MockMenu, isInitializing: true },
			provider: MockMenu,
			client: MockMenu,
			model: MockMenu,
		},
	},
};

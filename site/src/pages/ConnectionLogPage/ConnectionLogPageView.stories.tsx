import { chromaticWithTablet } from "testHelpers/chromatic";
import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockUserOwner,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	getDefaultFilterProps,
	MockMenu,
} from "components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import type { ComponentProps } from "react";
import { ConnectionLogPageView } from "./ConnectionLogPageView";

type FilterProps = ComponentProps<typeof ConnectionLogPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: `username:${MockUserOwner.username}`,
	values: {
		username: MockUserOwner.username,
		status: undefined,
		type: undefined,
		organization: undefined,
	},
	menus: {
		user: MockMenu,
		status: MockMenu,
		type: MockMenu,
	},
});

const meta: Meta<typeof ConnectionLogPageView> = {
	title: "pages/ConnectionLogPage",
	component: ConnectionLogPageView,
	args: {
		connectionLogs: [
			MockConnectedSSHConnectionLog,
			MockDisconnectedSSHConnectionLog,
		],
		isConnectionLogVisible: true,
		filterProps: defaultFilterProps,
	},
};

export default meta;
type Story = StoryObj<typeof ConnectionLogPageView>;

export const ConnectionLog: Story = {
	parameters: { chromatic: chromaticWithTablet },
	args: {
		connectionLogsQuery: mockSuccessResult,
	},
};

export const Loading: Story = {
	args: {
		connectionLogs: undefined,
		isNonInitialPage: false,
		connectionLogsQuery: mockInitialRenderResult,
	},
};

export const EmptyPage: Story = {
	args: {
		connectionLogs: [],
		isNonInitialPage: true,
		connectionLogsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NoLogs: Story = {
	args: {
		connectionLogs: [],
		isNonInitialPage: false,
		connectionLogsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NotVisible: Story = {
	args: {
		isConnectionLogVisible: false,
		connectionLogsQuery: mockInitialRenderResult,
	},
};

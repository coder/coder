import { chromaticWithTablet } from "testHelpers/chromatic";
import { MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { GlobalWorkspaceSession } from "api/typesGenerated";
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
	query: `workspace_owner:${MockUserOwner.username}`,
	values: {
		workspace_owner: MockUserOwner.username,
		organization: undefined,
	},
	menus: {
		user: MockMenu,
	},
});

const MockGlobalSession: GlobalWorkspaceSession = {
	id: "session-1",
	workspace_id: "workspace-1",
	workspace_name: "my-workspace",
	workspace_owner_username: "testuser",
	ip: "192.168.1.100",
	client_hostname: "dev-laptop",
	status: "ongoing",
	started_at: "2024-01-15T10:00:00Z",
	connections: [
		{
			ip: "192.168.1.100",
			status: "ongoing",
			created_at: "2024-01-15T10:00:00Z",
			connected_at: "2024-01-15T10:00:01Z",
			type: "ssh",
			client_hostname: "dev-laptop",
		},
	],
};

const MockEndedSession: GlobalWorkspaceSession = {
	id: "session-2",
	workspace_id: "workspace-2",
	workspace_name: "staging-env",
	workspace_owner_username: "admin",
	ip: "10.0.0.5",
	status: "clean_disconnected",
	started_at: "2024-01-15T08:00:00Z",
	ended_at: "2024-01-15T09:30:00Z",
	connections: [
		{
			ip: "10.0.0.5",
			status: "clean_disconnected",
			created_at: "2024-01-15T08:00:00Z",
			connected_at: "2024-01-15T08:00:01Z",
			ended_at: "2024-01-15T09:30:00Z",
			type: "vscode",
		},
	],
};

const meta: Meta<typeof ConnectionLogPageView> = {
	title: "pages/ConnectionLogPage",
	component: ConnectionLogPageView,
	args: {
		sessions: [MockGlobalSession, MockEndedSession],
		isConnectionLogVisible: true,
		filterProps: defaultFilterProps,
	},
};

export default meta;
type Story = StoryObj<typeof ConnectionLogPageView>;

export const Sessions: Story = {
	parameters: { chromatic: chromaticWithTablet },
	args: {
		sessionsQuery: mockSuccessResult,
	},
};

export const Loading: Story = {
	args: {
		sessions: undefined,
		isNonInitialPage: false,
		sessionsQuery: mockInitialRenderResult,
	},
};

export const EmptyPage: Story = {
	args: {
		sessions: [],
		isNonInitialPage: true,
		sessionsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NoSessions: Story = {
	args: {
		sessions: [],
		isNonInitialPage: false,
		sessionsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NotVisible: Story = {
	args: {
		isConnectionLogVisible: false,
		sessionsQuery: mockInitialRenderResult,
	},
};

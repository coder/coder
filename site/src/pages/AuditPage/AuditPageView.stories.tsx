import type { Meta, StoryObj } from "@storybook/react";
import {
	MockMenu,
	getDefaultFilterProps,
} from "components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import type { ComponentProps } from "react";
import { chromaticWithTablet } from "testHelpers/chromatic";
import {
	MockAuditLog,
	MockAuditLog2,
	MockAuditLog3,
	MockUserOwner,
} from "testHelpers/entities";
import { AuditPageView } from "./AuditPageView";

type FilterProps = ComponentProps<typeof AuditPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "owner:me",
	values: {
		username: MockUserOwner.username,
		action: undefined,
		resource_type: undefined,
		organization: undefined,
	},
	menus: {
		user: MockMenu,
		action: MockMenu,
		resourceType: MockMenu,
	},
});

const meta: Meta<typeof AuditPageView> = {
	title: "pages/AuditPage",
	component: AuditPageView,
	args: {
		auditLogs: [MockAuditLog, MockAuditLog2, MockAuditLog3],
		isAuditLogVisible: true,
		filterProps: defaultFilterProps,
		showOrgDetails: false,
	},
};

export default meta;
type Story = StoryObj<typeof AuditPageView>;

export const AuditPage: Story = {
	parameters: { chromatic: chromaticWithTablet },
	args: {
		auditsQuery: mockSuccessResult,
	},
};

export const Loading: Story = {
	args: {
		auditLogs: undefined,
		isNonInitialPage: false,
		auditsQuery: mockInitialRenderResult,
	},
};

export const EmptyPage: Story = {
	args: {
		auditLogs: [],
		isNonInitialPage: true,
		auditsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NoLogs: Story = {
	args: {
		auditLogs: [],
		isNonInitialPage: false,
		auditsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
		} as UsePaginatedQueryResult,
	},
};

export const NotVisible: Story = {
	args: {
		isAuditLogVisible: false,
		auditsQuery: mockInitialRenderResult,
	},
};

export const MultiOrg: Story = {
	parameters: { chromatic: chromaticWithTablet },
	args: {
		showOrgDetails: true,
		auditsQuery: mockSuccessResult,
		filterProps: {
			...defaultFilterProps,
			menus: {
				...defaultFilterProps.menus,
				organization: MockMenu,
			},
		},
	},
};

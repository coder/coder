import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, within } from "storybook/test";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockAuditLog,
	MockAuditLog2,
	MockAuditLog3,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
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
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof AuditPageView>;

export const AuditPage: Story = {
	parameters: { pixel: { matrix: pixelWithTablet } },
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Learn about Premium" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NotVisibleWithoutLicenseAccess: Story = {
	args: {
		...NotVisible.args,
		permissions: { ...MockPermissions, viewAllLicenses: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Learn about Premium" }),
		).not.toBeInTheDocument();
	},
};

export const MultiOrg: Story = {
	parameters: { pixel: { matrix: pixelWithTablet } },
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

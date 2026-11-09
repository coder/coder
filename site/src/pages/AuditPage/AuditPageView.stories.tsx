import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockAuditLog,
	MockAuditLog2,
	MockAuditLog3,
	MockDefaultOrganization,
	MockOrganization2,
	MockPermissions,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { docs } from "#/utils/docs";
import { AuditPageView } from "./AuditPageView";

type FilterProps = React.ComponentProps<typeof AuditPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "username:me",
	values: {
		username: "me",
		action: undefined,
		resource_type: undefined,
		organization: undefined,
	},
});

const meta: Meta<typeof AuditPageView> = {
	title: "pages/AuditPage",
	component: AuditPageView,
	decorators: [withDashboardProvider, withAuthProvider],
	parameters: { user: MockUserOwner },
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

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.getByRole("link", { name: /View docs/ }),
		).toHaveAttribute("href", docs("/admin/security/audit-logs"));
	},
};

export const NotVisibleWithoutLicenseAccess: Story = {
	args: {
		...NotVisible.args,
		permissions: {
			...MockPermissions,
			viewAllLicenses: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const FilterByChatInstructionSettings: Story = {
	args: {
		auditsQuery: mockSuccessResult,
		filterProps: {
			filter: {
				...defaultFilterProps.filter,
				query: "resource_type:chat_instruction_settings",
				values: { resource_type: "chat_instruction_settings" },
				used: true,
			},
		},
	},
};

export const ChatOperationalSettingsFilter: Story = {
	args: {
		auditsQuery: mockSuccessResult,
		filterProps: {
			filter: {
				...defaultFilterProps.filter,
				query: "resource_type:chat_operational_settings",
				values: { resource_type: "chat_operational_settings" },
				used: true,
			},
		},
	},
};

export const FilterMenuOpen: Story = {
	args: { auditsQuery: mockSuccessResult },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};

export const ResourceTypeMenuOpen: Story = {
	args: { auditsQuery: mockSuccessResult },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
		await userEvent.hover(
			await screen.findByRole("option", { name: "Resource type" }),
		);
	},
};

export const MultiOrg: Story = {
	parameters: {
		pixel: { matrix: pixelWithTablet },
		showOrganizations: true,
		queries: [
			{
				key: permittedOrganizationsKey({
					object: { resource_type: "audit_log" },
					action: "read",
				}),
				data: [MockDefaultOrganization, MockOrganization2],
			},
		],
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	args: {
		showOrgDetails: true,
		auditsQuery: mockSuccessResult,
	},
};

export const OrganizationMenuOpen: Story = {
	...MultiOrg,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
		await userEvent.hover(
			await screen.findByRole("option", { name: "Organization" }),
		);
	},
};

export const InvalidFilter: Story = {
	args: {
		auditsQuery: mockSuccessResult,
		filterProps: {
			filter: {
				...defaultFilterProps.filter,
				query: "action:invalid",
				values: { action: "invalid" },
				used: true,
			},
			error: mockApiError({
				message: "Invalid audit log query.",
				validations: [
					{ field: "action", detail: "Action must be a valid audit action." },
				],
			}),
		},
	},
};

export const SelectedPresetMenuOpen: Story = {
	...FilterMenuOpen,
	args: {
		auditsQuery: mockSuccessResult,
		filterProps: getDefaultFilterProps<FilterProps>({
			query: "resource_type:workspace action:create",
			values: {
				resource_type: "workspace",
				action: "create",
			},
		}),
	},
};

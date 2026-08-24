import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import type { ResourceType } from "#/api/typesGenerated";
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
import { docs } from "#/utils/docs";
import { useResourceTypeFilterMenu } from "./AuditFilter";
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

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.getByRole("link", { name: /Read the docs/ }),
		).toHaveAttribute("href", docs("/admin/security/audit-logs"));
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
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

const onResourceTypeChange = fn();

// Uses the real resource-type menu so generated resource types and their
// friendly labels are verified together.
const resourceTypeFilterStory = (
	label: string,
	value: ResourceType,
): Story => ({
	args: {
		auditsQuery: mockSuccessResult,
	},
	render: function AuditPageViewWithResourceTypeMenu(args) {
		const resourceTypeMenu = useResourceTypeFilterMenu({
			value: undefined,
			onChange: onResourceTypeChange,
		});
		return (
			<AuditPageView
				{...args}
				filterProps={{
					...defaultFilterProps,
					menus: {
						...defaultFilterProps.menus,
						resourceType: resourceTypeMenu,
					},
				}}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		onResourceTypeChange.mockClear();

		await userEvent.click(
			canvas.getByRole("button", { name: "Select a resource type" }),
		);
		const option = await screen.findByRole("option", { name: label });
		await userEvent.click(option);

		await expect(onResourceTypeChange).toHaveBeenCalledWith(
			expect.objectContaining({ value, label }),
		);
	},
});

export const FilterByChatInstructionSettings = resourceTypeFilterStory(
	"Chat Instruction Settings",
	"chat_instruction_settings",
);

export const ChatOperationalSettingsFilter = resourceTypeFilterStory(
	"Chat Operational Settings",
	"chat_operational_settings",
);

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

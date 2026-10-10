import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type {
	AgentHoursGroupAllotments,
	AgentHoursOrganizationAllotment,
} from "#/api/typesGenerated";
import {
	MockAgentHoursGroupAllotment,
	MockAgentHoursOrganizationAllotment,
	MockDefaultOrganization,
	MockGroup,
	MockGroup2,
	MockGroup3,
	MockOrganization2,
	MockOrganization3,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { AgentHoursPageView } from "./AgentHoursPageView";
import { OrganizationAgentHoursView } from "./OrganizationAgentHoursView";

const organizations = [
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
];

const mockAgentHoursOrganization2Allotment: AgentHoursOrganizationAllotment = {
	...MockAgentHoursOrganizationAllotment,
	organization_id: MockOrganization2.id,
	organization_name: MockOrganization2.name,
	organization_display_name: MockOrganization2.display_name,
	allotment_bps: 2550,
};

const mockGroupAllotments: AgentHoursGroupAllotments = {
	organization_allotment_bps: MockAgentHoursOrganizationAllotment.allotment_bps,
	groups: [
		MockAgentHoursGroupAllotment,
		{
			...MockAgentHoursGroupAllotment,
			group_id: MockGroup2.id,
			group_name: MockGroup2.name,
			group_display_name: MockGroup2.display_name,
			allotment_bps: 1500,
		},
	],
};

const mockOrganizationSectionProps: React.ComponentProps<
	typeof OrganizationAgentHoursView
> = {
	licenseHours: 1000,
	groupAllotments: mockGroupAllotments,
	groups: [MockGroup, MockGroup2, MockGroup3],
	error: null,
	onSave: fn(async () => undefined),
	onRemove: fn(async () => undefined),
};

const meta = {
	title: "pages/AISettingsPage/AgentHoursPage/AgentHoursPageView",
	component: AgentHoursPageView,
	args: {
		isLicensed: true,
		canEditDeploymentConfig: true,
		canManageAllotments: true,
		licenseHours: 1000,
		organizationAllotments: [
			MockAgentHoursOrganizationAllotment,
			mockAgentHoursOrganization2Allotment,
		],
		organizationAllotmentsError: null,
		organizations,
		onSaveOrganizationAllotment: fn(async () => undefined),
		onRemoveOrganizationAllotment: fn(async () => undefined),
		organization: MockDefaultOrganization,
		groupAllotmentOrganizations: organizations,
		onSelectOrganization: fn(),
		requestedOrganizationDenied: false,
		isOrganizationAccessLoading: false,
		organizationAccessError: null,
		organizationAgentHours: (
			<OrganizationAgentHoursView {...mockOrganizationSectionProps} />
		),
	},
} satisfies Meta<typeof AgentHoursPageView>;

export default meta;
type Story = StoryObj<typeof AgentHoursPageView>;

export const FiniteAllocation: Story = {};

export const UnlimitedAllocation: Story = {
	args: {
		licenseHours: undefined,
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				licenseHours={undefined}
			/>
		),
	},
};

export const NoAllotments: Story = {
	args: {
		organizationAllotments: [],
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={{ organization_allotment_bps: null, groups: [] }}
			/>
		),
	},
};

export const FullyAllotted: Story = {
	args: {
		organizationAllotments: [
			{ ...MockAgentHoursOrganizationAllotment, allotment_bps: 7000 },
			{ ...mockAgentHoursOrganization2Allotment, allotment_bps: 3000 },
		],
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={{
					...mockGroupAllotments,
					groups: [{ ...MockAgentHoursGroupAllotment, allotment_bps: 10000 }],
				}}
			/>
		),
	},
};

export const OrganizationWithoutAllotment: Story = {
	args: {
		organizationAllotments: [mockAgentHoursOrganization2Allotment],
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={{
					organization_allotment_bps: null,
					groups: mockGroupAllotments.groups,
				}}
			/>
		),
	},
};

const longName =
	"PlatformEngineeringInfrastructureReliabilityOperationsNorthAmer";

export const LongNamesAndSmallShares: Story = {
	args: {
		organizationAllotments: [
			{
				...MockAgentHoursOrganizationAllotment,
				organization_display_name: longName,
				allotment_bps: 1,
			},
		],
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={{
					organization_allotment_bps: 1,
					groups: [
						{
							...MockAgentHoursGroupAllotment,
							group_display_name: longName,
							allotment_bps: 1,
						},
					],
				}}
			/>
		),
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};

export const Unlicensed: Story = {
	args: { isLicensed: false },
};

export const UnlicensedAccessLoading: Story = {
	args: {
		isLicensed: false,
		canEditDeploymentConfig: false,
		canManageAllotments: false,
		organization: undefined,
		groupAllotmentOrganizations: [],
		isOrganizationAccessLoading: true,
	},
};

export const UnlicensedAccessError: Story = {
	args: {
		isLicensed: false,
		canEditDeploymentConfig: false,
		canManageAllotments: false,
		organization: undefined,
		groupAllotmentOrganizations: [],
		organizationAccessError: mockApiError({
			message: "Failed to load organizations.",
		}),
	},
};

export const GroupManager: Story = {
	args: {
		canEditDeploymentConfig: false,
		organizationAllotments: undefined,
		groupAllotmentOrganizations: [MockDefaultOrganization],
	},
};

export const Loading: Story = {
	args: {
		organizationAllotments: undefined,
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={undefined}
				groups={undefined}
			/>
		),
	},
};

export const LoadError: Story = {
	args: {
		organizationAllotments: undefined,
		organizationAllotmentsError: mockApiError({
			message: "Failed to load Agent Hours allotments.",
		}),
	},
};

export const AddAllotmentDialog: Story = {
	play: async ({ canvasElement }) => {
		const section = within(canvasElement).getByRole("region", {
			name: "Organization allotments",
		});
		await userEvent.click(
			within(section).getByRole("button", { name: "Add allotment" }),
		);
	},
};

export const GroupAllotmentsLoadError: Story = {
	args: {
		organizationAgentHours: (
			<OrganizationAgentHoursView
				{...mockOrganizationSectionProps}
				groupAllotments={undefined}
				groups={undefined}
				error={mockApiError({ message: "Failed to load group allotments." })}
			/>
		),
	},
};

export const EditAllotmentDialog: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Edit allotment for ${MockOrganization2.display_name}`,
			}),
		);
	},
};

export const NegativeAllotment: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Edit allotment for ${MockOrganization2.display_name}`,
			}),
		);
		const input = screen.getByRole("textbox", { name: "Allotment" });
		await userEvent.clear(input);
		await userEvent.type(input, "-1");
		await userEvent.click(screen.getByRole("button", { name: "Save" }));
	},
};

export const RemoveAllotmentDialog: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Remove allotment for ${MockOrganization2.display_name}`,
			}),
		);
	},
};

export const AllotmentDialogServerError: Story = {
	args: {
		onSaveOrganizationAllotment: fn(() =>
			Promise.reject(
				mockApiError({
					message: "Agent Hours allotments cannot exceed 100% in total.",
					detail: "Only 10% of the deployment's Agent Hours is unallotted.",
				}),
			),
		),
	},
	play: async ({ canvasElement }) => {
		const section = within(canvasElement).getByRole("region", {
			name: "Organization allotments",
		});
		await userEvent.click(
			within(section).getByRole("button", { name: "Add allotment" }),
		);
		await userEvent.click(
			screen.getByRole("combobox", { name: "Organization" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: MockOrganization3.display_name,
			}),
		);
		// The closing list keeps the rest of the dialog hidden until it unmounts.
		await userEvent.type(
			await screen.findByRole("textbox", { name: "Allotment" }),
			"10",
		);
		await userEvent.click(screen.getByRole("button", { name: "Save" }));
	},
};

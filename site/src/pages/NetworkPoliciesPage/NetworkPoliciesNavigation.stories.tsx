import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import { networkPolicyTemplatesKey } from "#/api/queries/networkPolicies";
import { MockOrganization, MockTemplate } from "#/testHelpers/entities";
import { NetworkPoliciesNavigation } from "./NetworkPoliciesNavigation";

const mockEngineeringOrganization = {
	...MockOrganization,
	id: "20000000-0000-4000-8000-000000000002",
	name: "engineering",
	display_name: "Engineering",
};
const mockEngineeringTemplate = {
	...MockTemplate,
	id: "40000000-0000-4000-8000-000000000003",
	organization_id: mockEngineeringOrganization.id,
};

const meta = {
	title: "pages/NetworkPolicies/Navigation",
	component: NetworkPoliciesNavigation,
	args: { organizations: [MockOrganization], templateUpdatePermissions: {} },
} satisfies Meta<typeof NetworkPoliciesNavigation>;
export default meta;
type Story = StoryObj<typeof meta>;

export const MultipleOrganizations: Story = {
	args: { organizations: [MockOrganization, mockEngineeringOrganization] },
	parameters: {
		queries: [
			{
				key: networkPolicyTemplatesKey(MockOrganization.name),
				data: [MockTemplate],
			},
			{
				key: networkPolicyTemplatesKey(mockEngineeringOrganization.name),
				data: [mockEngineeringTemplate],
			},
		],
	},
};
export const MultipleOrganizationsMobile: Story = {
	args: { organizations: [MockOrganization, mockEngineeringOrganization] },
	globals: { viewport: { value: "iphone12", isRotated: false } },
	parameters: {
		queries: [
			{
				key: networkPolicyTemplatesKey(MockOrganization.name),
				data: [MockTemplate],
			},
			{
				key: networkPolicyTemplatesKey(mockEngineeringOrganization.name),
				data: [mockEngineeringTemplate],
			},
		],
	},
};
export const TemplateAdmin: Story = {
	parameters: {
		queries: [
			{
				key: networkPolicyTemplatesKey(MockOrganization.name),
				data: [MockTemplate],
			},
		],
	},
};
export const OrganizationAdminWithoutTemplates: Story = {
	args: { templateUpdatePermissions: { [MockOrganization.id]: true } },
	parameters: {
		queries: [
			{ key: networkPolicyTemplatesKey(MockOrganization.name), data: [] },
		],
	},
};
export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyTemplates").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyTemplates").mockRejectedValue({
			message: "Unable to check network policy access.",
			detail: "Retry to load administrable templates.",
		});
	},
};
export const OrganizationAdminWithLookupError: Story = {
	...LoadError,
	args: { templateUpdatePermissions: { [MockOrganization.id]: true } },
};
export const NoAccess: Story = {
	parameters: {
		queries: [
			{ key: networkPolicyTemplatesKey(MockOrganization.name), data: [] },
		],
	},
};

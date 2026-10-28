import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	foundationPermissionsKey,
	foundationsKey,
} from "#/api/queries/foundations";
import {
	MockDefaultOrganization,
	MockOrganization2,
	mockApiError,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import {
	MockFoundation,
	MockFoundationPermissions,
} from "#/testHelpers/templateResources";
import FoundationsPage from "./FoundationsPage";

const mockSecondOrganizationFoundation = {
	...MockFoundation,
	id: "second-foundation",
	name: "production-network",
	organization_id: MockOrganization2.id,
	template_id: null,
};
const mockUnattachedFoundation = { ...MockFoundation, template_id: null };

const meta = {
	title: "pages/Foundations/List",
	component: FoundationsPage,
	decorators: [withDashboardProvider],
	parameters: {
		experiments: ["template-resources"],
		reactRouter: reactRouterParameters({
			location: {
				pathParams: { organization: MockFoundation.organization_id },
			},
			routing: { path: "/organizations/:organization/foundations" },
		}),
		queries: [
			{
				key: foundationPermissionsKey(MockFoundation.organization_id),
				data: MockFoundationPermissions,
			},
			{
				key: foundationsKey(MockFoundation.organization_id),
				data: [MockFoundation],
			},
		],
	},
} satisfies Meta<typeof FoundationsPage>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Attached: Story = {};
export const AllOrganizations: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({ routing: { path: "/foundations" } }),
		queries: [
			{
				key: foundationsKey(MockDefaultOrganization.id),
				data: [MockFoundation],
			},
			{
				key: foundationsKey(MockOrganization2.id),
				data: [mockSecondOrganizationFoundation],
			},
			{
				key: foundationPermissionsKey(MockDefaultOrganization.id),
				data: MockFoundationPermissions,
			},
			{
				key: foundationPermissionsKey(MockOrganization2.id),
				data: MockFoundationPermissions,
			},
		],
	},
};
export const ExperimentDisabled: Story = { parameters: { experiments: [] } };
export const Unattached: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockFoundation.organization_id),
				data: MockFoundationPermissions,
			},
			{
				key: foundationsKey(MockFoundation.organization_id),
				data: [mockUnattachedFoundation],
			},
		],
	},
};
export const Empty: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockFoundation.organization_id),
				data: MockFoundationPermissions,
			},
			{ key: foundationsKey(MockFoundation.organization_id), data: [] },
		],
	},
};
export const Loading: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockFoundation.organization_id),
				data: MockFoundationPermissions,
			},
		],
	},
	beforeEach: () => {
		const mock = spyOn(API, "getFoundations").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const LoadError: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockFoundation.organization_id),
				data: MockFoundationPermissions,
			},
		],
	},
	beforeEach: () => {
		const mock = spyOn(API, "getFoundations").mockRejectedValue(
			mockApiError({ message: "Cannot load Foundations." }),
		);
		return () => mock.mockRestore();
	},
};

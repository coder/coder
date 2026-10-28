import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { foundationPermissionsKey } from "#/api/queries/foundations";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { MockFoundationPermissions } from "#/testHelpers/templateResources";
import CreateFoundationPage from "./CreateFoundationPage";

const mockNoCreateFoundationPermissions = {
	...MockFoundationPermissions,
	canCreateFoundation: false,
};

const meta = {
	title: "pages/Foundations/Create",
	component: CreateFoundationPage,
	decorators: [withDashboardProvider],
	parameters: {
		experiments: ["template-resources"],
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			routing: { path: "/foundations/new" },
			location: { searchParams: { organization: MockOrganization2.id } },
		}),
		queries: [
			{
				key: foundationPermissionsKey(MockOrganization2.id),
				data: MockFoundationPermissions,
			},
		],
	},
} satisfies Meta<typeof CreateFoundationPage>;
export default meta;
type Story = StoryObj<typeof meta>;
export const PreselectedOrganization: Story = {};
export const PermissionsLoading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		const mock = spyOn(API, "checkAuthorization").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const PermissionsError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		const mock = spyOn(API, "checkAuthorization").mockRejectedValue(
			new Error("Unable to check Foundation permissions."),
		);
		return () => mock.mockRestore();
	},
};
export const CreateError: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "createFoundation").mockRejectedValue(
			new Error("Unable to create Foundation. Try again."),
		);
		return () => mock.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("textbox", { name: "Foundation name" }),
			"shared-network",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Create Foundation" }),
		);
	},
};
export const NoPermission: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(MockOrganization2.id),
				data: mockNoCreateFoundationPermissions,
			},
		],
	},
};
export const NoOrganizations: Story = {
	parameters: { organizations: [], queries: [] },
};
export const ExperimentDisabled: Story = { parameters: { experiments: [] } };

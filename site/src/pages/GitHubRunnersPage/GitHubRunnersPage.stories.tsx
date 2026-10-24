import type { Meta, StoryObj } from "@storybook/react-vite";
import { focusManager } from "react-query";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import { githubRunnerPoolsKey } from "#/api/queries/githubRunners";
import { getOrganizationsPermissionsKey } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockGitHubRunnerPool,
	MockNoOrganizationPermissions,
	MockOrganization2,
	MockOrganizationPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import GitHubRunnersPage from "./GitHubRunnersPage";

const meta = {
	title: "pages/GitHubRunnersPage/Page",
	component: GitHubRunnersPage,
	decorators: [withDashboardProvider, withAuthProvider],
	parameters: { user: MockUserOwner },
	beforeEach: () => {
		// Keep polling from replacing the seeded visual states during capture.
		focusManager.setFocused(false);
		return () => focusManager.setFocused(undefined);
	},
} satisfies Meta<typeof GitHubRunnersPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const PermissionLoading: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const PermissionError: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockRejectedValue(
			new Error("Unable to check runner-pool permissions."),
		);
	},
};
export const NoAccess: Story = {
	parameters: {
		queries: [
			{
				key: getOrganizationsPermissionsKey([MockDefaultOrganization.id]),
				data: { [MockDefaultOrganization.id]: MockNoOrganizationPermissions },
			},
		],
	},
};
export const MultipleOrganizations: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: getOrganizationsPermissionsKey([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]),
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
};
export const Mobile: Story = {
	parameters: {
		pixel: { matrix: pixelWithPhone },
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: getOrganizationsPermissionsKey([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]),
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	globals: { viewport: { value: "iphone12" } },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	MockOrganization,
	MockPermissions,
	mockApiError,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { docs } from "#/utils/docs";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const meta: Meta<typeof CreateOrganizationPageView> = {
	title: "pages/OrganizationSettingsPage/CreateOrganizationPage",
	component: CreateOrganizationPageView,
	decorators: [withToaster],
	args: {
		isEntitled: true,
		permissions: MockPermissions,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/organizations/new" },
			routing: [
				{ path: "/organizations", useStoryElement: true },
				{ path: "/organizations/new", useStoryElement: true },
				{ path: "/organizations/:organization", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof CreateOrganizationPageView>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "New Organization" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("form", { name: "Organization settings form" }),
		).toBeVisible();
	},
};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "New Organization" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("form", { name: "Organization settings form" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /View docs/ }),
		).toHaveAttribute("href", docs("/admin/users/organizations"));
		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NotEntitledWithoutLicenseAccess: Story = {
	args: {
		isEntitled: false,
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

export const WithError: Story = {
	beforeEach: () => {
		spyOn(API, "createOrganization").mockRejectedValue(
			mockApiError({ message: "Oh no!" }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.type(canvas.getByLabelText(/slug/i), "new-org");
		await user.click(
			canvas.getByRole("button", { name: "Create organization" }),
		);

		const alert = await canvas.findByRole("alert");
		await expect(within(alert).getByText("Oh no!")).toBeVisible();
	},
};

export const InvalidName: Story = {
	beforeEach: () => {
		spyOn(API, "createOrganization").mockRejectedValue(
			mockApiError({
				message: "Display name is bad",
				validations: [
					{
						field: "display_name",
						detail: "That display name is terrible. What were you thinking?",
					},
				],
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.type(canvas.getByLabelText(/slug/i), "new-org");
		await user.type(canvas.getByLabelText("Display name"), "Bad Name");
		await user.click(
			canvas.getByRole("button", { name: "Create organization" }),
		);

		await expect(
			canvas.findByText(
				"That display name is terrible. What were you thinking?",
			),
		).resolves.toBeVisible();
	},
};

export const CreatesOrganization: Story = {
	beforeEach: () => {
		spyOn(API, "createOrganization").mockResolvedValue({
			...MockOrganization,
			name: "new-org",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		await user.type(canvas.getByLabelText(/slug/i), "new-org");
		await user.click(
			canvas.getByRole("button", { name: "Create organization" }),
		);

		await expect(
			body.findByText('Organization "new-org" created successfully.'),
		).resolves.toBeInTheDocument();
	},
};

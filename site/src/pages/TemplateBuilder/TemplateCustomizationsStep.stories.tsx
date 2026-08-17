import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import {
	getProvisionerDaemonsKey,
	permittedOrganizationsKey,
} from "#/api/queries/organizations";
import type { AuthorizationCheck } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { TemplateCustomizationsStep } from "./TemplateCustomizationsStep";
import { initialWizardState } from "./wizardState";

const permittedOrgsCheck: AuthorizationCheck = {
	object: { resource_type: "template" },
	action: "create",
};
const permittedOrgsKey = permittedOrganizationsKey(permittedOrgsCheck);

const meta: Meta<typeof TemplateCustomizationsStep> = {
	title: "pages/TemplateBuilder/TemplateCustomizationsStep",
	component: TemplateCustomizationsStep,
	args: {
		state: {
			...initialWizardState,
			selectedBase: {
				id: "docker",
				name: "Docker Containers",
				iconUrl: "/icon/docker.svg",
				hasParameters: false,
				hasPrerequisites: false,
			},
			name: "my-template",
			displayName: "My Template",
		},
		onChangeField: fn(),
		onProvisionerStatusChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TemplateCustomizationsStep>;

export const RequiresOrganizationSelection: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
			{
				key: getProvisionerDaemonsKey(MockOrganization2.id),
				data: [{ id: "provisioner-1" }],
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const organizationPicker = canvas.getByRole("button", {
			name: /organization/i,
		});

		await expect(organizationPicker).toHaveAttribute("aria-required", "true");

		await userEvent.click(organizationPicker);
		// Popover content portals outside the canvas root.
		const org2 = await screen.findByText(MockOrganization2.display_name);
		await userEvent.click(org2);

		await waitFor(() =>
			expect(args.onChangeField).toHaveBeenCalledWith(
				"organizationId",
				MockOrganization2.id,
			),
		);
	},
};

export const HidesPickerForSingleOrganization: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockDefaultOrganization],
			},
			{
				key: getProvisionerDaemonsKey(MockDefaultOrganization.id),
				data: [{ id: "provisioner-1" }],
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.queryByRole("button", { name: /organization/i }),
		).not.toBeInTheDocument();

		await waitFor(() =>
			expect(args.onChangeField).toHaveBeenCalledWith(
				"organizationId",
				MockDefaultOrganization.id,
			),
		);
	},
};

export const NoPermittedOrganizations: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrgsKey,
				data: [],
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.queryByRole("button", { name: /organization/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByText(/do not have permission to create templates/i),
		).toBeVisible();
		await expect(args.onChangeField).not.toHaveBeenCalledWith(
			"organizationId",
			expect.anything(),
		);
	},
};

export const FailsToLoadOrganizations: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizations").mockRejectedValue(
			new Error("failed to load organizations"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			await canvas.findByText("Failed to load organizations."),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /retry/i }),
		).toBeInTheDocument();
	},
};

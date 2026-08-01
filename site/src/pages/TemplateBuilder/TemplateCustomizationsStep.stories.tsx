import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { getProvisionerDaemonsKey } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { TemplateCustomizationsStep } from "./TemplateCustomizationsStep";
import { initialWizardState } from "./wizardState";

const permittedOrgsKey = [
	"organizations",
	"permitted",
	{ object: { resource_type: "template" }, action: "create" },
];

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
		const organizationPicker = canvas.getByTestId("organization-autocomplete");

		await expect(organizationPicker).toHaveAttribute("aria-required", "true");
		await expect(organizationPicker).toHaveTextContent(
			"Select an organization",
		);

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
			canvas.queryByTestId("organization-autocomplete"),
		).not.toBeInTheDocument();

		await waitFor(() =>
			expect(args.onChangeField).toHaveBeenCalledWith(
				"organizationId",
				MockDefaultOrganization.id,
			),
		);
	},
};

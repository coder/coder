import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	getProvisionerDaemonsKey,
	permittedOrganizations,
} from "#/api/queries/organizations";
import { Button } from "#/components/Button/Button";
import {
	MockDefaultOrganization,
	MockOrganization,
	MockOrganization2,
	MockProvisioner,
} from "#/testHelpers/entities";
import {
	TEMPLATE_CUSTOMIZATIONS_FORM_ID,
	TemplateCustomizationsStep,
} from "./TemplateCustomizationsStep";
import {
	initialWizardState,
	type TemplateBuilderWizardState,
} from "./wizardState";

const baseState: TemplateBuilderWizardState = {
	...initialWizardState,
	baseTemplateId: "docker",
	selectedBase: {
		id: "docker",
		name: "Docker Containers",
		iconUrl: "/icon/docker.svg",
		hasParameters: false,
		hasPrerequisites: false,
	},
	name: "docker",
	displayName: "Docker Containers",
	description: "Run workspaces as Docker containers",
	icon: "/icon/docker.svg",
};

const permittedOrgsKey = permittedOrganizations({
	object: { resource_type: "template" },
	action: "create",
}).queryKey;

const provisionersKey = (organizationId: string) =>
	getProvisionerDaemonsKey(organizationId);

const meta: Meta<typeof TemplateCustomizationsStep> = {
	title: "pages/TemplateBuilder/TemplateCustomizationsStep",
	component: TemplateCustomizationsStep,
	args: {
		state: baseState,
		onCreate: fn(),
		onProvisionerStatusChange: fn(),
	},
	parameters: {
		queries: [
			{ key: permittedOrgsKey, data: [MockOrganization, MockOrganization2] },
		],
	},
	// The "Create Template" submit button lives in the wizard's shared nav bar,
	// outside this component. It is associated with the form via the `form`
	// attribute, so the stories render an equivalent button to exercise submit.
	decorators: [
		(Story) => (
			<div className="flex flex-col gap-6">
				<Story />
				<Button type="submit" form={TEMPLATE_CUSTOMIZATIONS_FORM_ID}>
					Create Template
				</Button>
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof TemplateCustomizationsStep>;

export const MultipleOrganizations: Story = {};

// Submitting without choosing an organization surfaces an aggregated error at
// the top of the step instead of an inline field error, and does not call
// onCreate.
export const MissingOrganizationError: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await canvas.findByTestId("organization-autocomplete");
		await userEvent.click(
			canvas.getByRole("button", { name: "Create Template" }),
		);
		await canvas.findByText("Select an organization to continue.");
		await expect(args.onCreate).not.toHaveBeenCalled();
	},
};

// A single permitted organization is auto-selected, so a valid form submits and
// forwards the selected organization id to onCreate.
export const SingleOrganizationSubmits: Story = {
	parameters: {
		queries: [
			{ key: permittedOrgsKey, data: [MockDefaultOrganization] },
			{
				key: provisionersKey(MockDefaultOrganization.id),
				data: [MockProvisioner],
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		// Wait for the auto-selected org to render in the autocomplete.
		await canvas.findByText(MockDefaultOrganization.display_name);
		await userEvent.click(
			canvas.getByRole("button", { name: "Create Template" }),
		);
		await expect(args.onCreate).toHaveBeenCalledWith(
			expect.objectContaining({
				organization_id: MockDefaultOrganization.id,
				name: "docker",
			}),
		);
	},
};

// A missing template ID surfaces the name validation message at the top of the
// step and blocks submission.
export const MissingNameError: Story = {
	args: {
		state: { ...baseState, name: "" },
	},
	parameters: {
		queries: [
			{ key: permittedOrgsKey, data: [MockDefaultOrganization] },
			{
				key: provisionersKey(MockDefaultOrganization.id),
				data: [MockProvisioner],
			},
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(MockDefaultOrganization.display_name);
		await userEvent.click(
			canvas.getByRole("button", { name: "Create Template" }),
		);
		await canvas.findByText("Please enter a template id.");
		await expect(args.onCreate).not.toHaveBeenCalled();
	},
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { getProvisionerDaemonsKey } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockTemplate,
	MockTemplateExample,
	MockTemplateVersionVariable1,
	MockTemplateVersionVariable2,
	MockTemplateVersionVariable3,
	MockTemplateVersionVariable4,
	MockTemplateVersionVariable5,
} from "#/testHelpers/entities";
import { CreateTemplateForm } from "./CreateTemplateForm";

const meta: Meta<typeof CreateTemplateForm> = {
	title: "pages/CreateTemplatePage/CreateTemplateForm",
	component: CreateTemplateForm,
	args: {
		isSubmitting: false,
		onCancel: action("onCancel"),
	},
};

export default meta;
type Story = StoryObj<typeof CreateTemplateForm>;

export const Upload: Story = {
	args: {
		upload: {
			isUploading: false,
			onRemove: () => {},
			onUpload: () => {},
			file: undefined,
		},
	},
};

export const UploadWithOrgPicker: Story = {
	args: {
		...Upload.args,
		showOrganizationPicker: true,
	},
};

export const StarterTemplate: Story = {
	args: {
		starterTemplate: MockTemplateExample,
	},
};

export const StarterTemplateWithOrgPicker: Story = {
	args: {
		...StarterTemplate.args,
		showOrganizationPicker: true,
	},
};

// Query key used by permittedOrganizations() in the form.
const permittedOrgsKey = [
	"organizations",
	"permitted",
	{ object: { resource_type: "template" }, action: "create" },
];

export const StarterTemplateWithProvisionerWarning: Story = {
	parameters: {
		queries: [
			{
				key: permittedOrgsKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
			{
				key: getProvisionerDaemonsKey(MockOrganization2.id),
				data: [],
			},
		],
	},
	args: {
		...StarterTemplate.args,
		showOrganizationPicker: true,
	},
	play: async () => {
		const organizationPicker = screen.getByTestId("organization-autocomplete");
		await userEvent.click(organizationPicker);
		const org2 = await screen.findByText(MockOrganization2.display_name);
		await userEvent.click(org2);
	},
};

export const StarterTemplatePermissionsCheck: Story = {
	parameters: {
		queries: [
			{
				// Only MockDefaultOrganization passes the permission
				// check; MockOrganization2 is filtered out by the
				// permittedOrganizations query.
				key: permittedOrgsKey,
				data: [MockDefaultOrganization],
			},
		],
	},
	args: {
		...StarterTemplate.args,
		showOrganizationPicker: true,
	},
	play: async () => {
		// When only one org passes the permission check, it should be
		// auto-selected in the picker.
		const organizationPicker = screen.getByTestId("organization-autocomplete");
		await waitFor(() =>
			expect(organizationPicker).toHaveTextContent(
				MockDefaultOrganization.display_name,
			),
		);
		await userEvent.click(organizationPicker);
	},
};

export const DuplicateTemplateWithVariables: Story = {
	args: {
		copiedTemplate: MockTemplate,
		variables: [
			MockTemplateVersionVariable1,
			MockTemplateVersionVariable2,
			MockTemplateVersionVariable3,
			MockTemplateVersionVariable4,
			MockTemplateVersionVariable5,
		],
	},
};

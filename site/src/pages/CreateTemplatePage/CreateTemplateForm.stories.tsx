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
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	getProvisionerDaemonsKey,
	organizationsKey,
} from "api/queries/organizations";
import { action } from "storybook/actions";
import { screen, userEvent } from "storybook/test";
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

const canCreateTemplate = (organizationId: string) => {
	return {
		[organizationId]: {
			object: {
				resource_type: "template",
				organization_id: organizationId,
			},
			action: "create",
		},
	};
};

export const StarterTemplateWithProvisionerWarning: Story = {
	parameters: {
		queries: [
			{
				key: organizationsKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
			{
				key: [
					"authorization",
					{
						checks: {
							...canCreateTemplate(MockDefaultOrganization.id),
							...canCreateTemplate(MockOrganization2.id),
						},
					},
				],
				data: {
					[MockDefaultOrganization.id]: true,
					[MockOrganization2.id]: true,
				},
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
		const organizationPicker = screen.getByPlaceholderText("Organization name");
		await userEvent.click(organizationPicker);
		const org2 = await screen.findByText(MockOrganization2.display_name);
		await userEvent.click(org2);
	},
};

export const StarterTemplatePermissionsCheck: Story = {
	parameters: {
		queries: [
			{
				key: organizationsKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
			{
				key: [
					"authorization",
					{
						checks: {
							...canCreateTemplate(MockDefaultOrganization.id),
							...canCreateTemplate(MockOrganization2.id),
						},
					},
				],
				data: {
					[MockDefaultOrganization.id]: true,
					[MockOrganization2.id]: false,
				},
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
		const organizationPicker = screen.getByPlaceholderText("Organization name");
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

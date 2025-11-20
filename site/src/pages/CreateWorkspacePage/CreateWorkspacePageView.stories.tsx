import { chromatic } from "testHelpers/chromatic";
import {
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter3,
	MockUserOwner,
	mockApiError,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { action } from "storybook/actions";
import { expect, screen, waitFor } from "storybook/test";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";

const meta: Meta<typeof CreateWorkspacePageView> = {
	title: "pages/CreateWorkspacePage",
	parameters: { chromatic },
	component: CreateWorkspacePageView,
	args: {
		defaultName: "",
		defaultOwner: MockUserOwner,
		autofillParameters: [],
		template: MockTemplate,
		parameters: [],
		presets: [],
		externalAuth: [],
		hasAllRequiredExternalAuth: true,
		mode: "form",
		permissions: {
			createWorkspaceForAny: true,
			canUpdateTemplate: false,
		},
		onCancel: action("onCancel"),
		templatePermissions: { canUpdateTemplate: true },
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof CreateWorkspacePageView>;

export const NoParameters: Story = {};

export const CreateWorkspaceError: Story = {
	args: {
		error: mockApiError({
			message:
				'Workspace "test" already exists in the "docker-amd64" template.',
			validations: [
				{
					field: "name",
					detail: "This value is already in use and should be unique.",
				},
			],
		}),
	},
};

export const SpecificVersion: Story = {
	args: {
		versionId: "specific-version",
	},
};

export const Duplicate: Story = {
	args: {
		mode: "duplicate",
	},
};

export const Parameters: Story = {
	args: {
		parameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter3,
			{
				name: "Region",
				required: false,
				description: "",
				description_plaintext: "",
				type: "string",
				form_type: "radio",
				mutable: false,
				default_value: "",
				icon: "/emojis/1f30e.png",
				options: [
					{
						name: "Pittsburgh",
						description: "",
						value: "us-pittsburgh",
						icon: "/emojis/1f1fa-1f1f8.png",
					},
					{
						name: "Helsinki",
						description: "",
						value: "eu-helsinki",
						icon: "/emojis/1f1eb-1f1ee.png",
					},
					{
						name: "Sydney",
						description: "",
						value: "ap-sydney",
						icon: "/emojis/1f1e6-1f1fa.png",
					},
				],
				ephemeral: false,
			},
		],
		autofillParameters: [
			{
				name: "first_parameter",
				value: "Cool suggestion",
				source: "user_history",
			},
			{
				name: "third_parameter",
				value: "aaaa",
				source: "url",
			},
		],
	},
};

export const PresetsButNoneSelected: Story = {
	args: {
		presets: [
			{
				ID: "preset-1",
				Name: "Preset 1",
				Description: "Preset 1 description",
				Icon: "/emojis/0031-fe0f-20e3.png",
				Default: false,
				Parameters: [
					{
						Name: MockTemplateVersionParameter1.name,
						Value: "preset 1 override",
					},
				],
				DesiredPrebuildInstances: null,
			},
			{
				ID: "preset-2",
				Name: "Preset 2",
				Description: "Preset 2 description",
				Icon: "/emojis/0032-fe0f-20e3.png",
				Default: false,
				Parameters: [
					{
						Name: MockTemplateVersionParameter2.name,
						Value: "42",
					},
				],
				DesiredPrebuildInstances: null,
			},
		],
		parameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter3,
		],
	},
};

export const PresetSelected: Story = {
	args: PresetsButNoneSelected.args,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Select a preset
		await userEvent.click(canvas.getByRole("button", { name: "None" }));
		await userEvent.click(screen.getByText("Preset 1"));
	},
};

export const PresetSelectedWithVisibleParameters: Story = {
	args: PresetsButNoneSelected.args,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Select a preset
		await userEvent.click(canvas.getByRole("button", { name: "None" }));
		await userEvent.click(screen.getByText("Preset 1"));
		// Toggle off the show preset parameters switch
		await userEvent.click(canvas.getByLabelText("Show preset parameters"));
	},
};

export const PresetReselected: Story = {
	args: PresetsButNoneSelected.args,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// First selection of Preset 1
		await userEvent.click(canvas.getByRole("button", { name: "None" }));
		await userEvent.click(screen.getByText("Preset 1"));

		// Reselect the same preset
		await userEvent.click(canvas.getByRole("button", { name: "Preset 1" }));
		await userEvent.click(canvas.getByText("Preset 1"));
	},
};

export const PresetNoneSelected: Story = {
	args: {
		...PresetsButNoneSelected.args,
		onSubmit: (request, owner) => {
			action("onSubmit")(request, owner);
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// First select a preset to set the field value
		await userEvent.click(canvas.getByRole("button", { name: "None" }));
		await userEvent.click(screen.getByText("Preset 1"));

		// Then select "None" to unset the field value
		await userEvent.click(screen.getByText("None"));

		// Fill in required fields and submit to test the API call
		await userEvent.type(
			canvas.getByLabelText("Workspace Name"),
			"test-workspace",
		);
		await userEvent.click(canvas.getByText("Create workspace"));
	},
	parameters: {
		docs: {
			description: {
				story:
					"This story tests that when 'None' preset is selected, the template_version_preset_id field is not included in the form submission. The story first selects a preset to set the field value, then selects 'None' to unset it, and finally submits the form to verify the API call behavior.",
			},
		},
	},
};

export const PresetsWithDefault: Story = {
	args: {
		presets: [
			{
				ID: "preset-1",
				Name: "Preset 1",
				Description: "Preset 1 description",
				Icon: "/emojis/0031-fe0f-20e3.png",
				Default: false,
				Parameters: [
					{
						Name: MockTemplateVersionParameter1.name,
						Value: "preset 1 override",
					},
				],
				DesiredPrebuildInstances: null,
			},
			{
				ID: "preset-2",
				Name: "Preset 2",
				Description: "Preset 2 description",
				Icon: "/emojis/0032-fe0f-20e3.png",
				Default: true,
				Parameters: [
					{
						Name: MockTemplateVersionParameter2.name,
						Value: "150189",
					},
				],
				DesiredPrebuildInstances: null,
			},
		],
		parameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter3,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should have the default preset listed first
		await waitFor(() =>
			expect(canvas.getByRole("button", { name: "Preset 2 (Default)" })),
		);
		// Wait for the switch to be available since preset parameters are populated asynchronously
		await canvas.findByLabelText("Show preset parameters");
		// Toggle off the show preset parameters switch
		await userEvent.click(canvas.getByLabelText("Show preset parameters"));
	},
};

export const ExternalAuth: Story = {
	args: {
		externalAuth: [
			{
				id: "github",
				type: "github",
				authenticated: false,
				authenticate_url: "",
				display_icon: "/icon/github.svg",
				display_name: "GitHub",
			},
			{
				id: "gitlab",
				type: "gitlab",
				authenticated: true,
				authenticate_url: "",
				display_icon: "/icon/gitlab.svg",
				display_name: "GitLab",
				optional: true,
			},
		],
		hasAllRequiredExternalAuth: false,
	},
};

export const ExternalAuthError: Story = {
	args: {
		error: true,
		externalAuth: [
			{
				id: "github",
				type: "github",
				authenticated: false,
				authenticate_url: "",
				display_icon: "/icon/github.svg",
				display_name: "GitHub",
			},
			{
				id: "gitlab",
				type: "gitlab",
				authenticated: false,
				authenticate_url: "",
				display_icon: "/icon/gitlab.svg",
				display_name: "GitLab",
				optional: true,
			},
		],
		hasAllRequiredExternalAuth: false,
	},
};

export const ExternalAuthAllRequiredConnected: Story = {
	args: {
		externalAuth: [
			{
				id: "github",
				type: "github",
				authenticated: true,
				authenticate_url: "",
				display_icon: "/icon/github.svg",
				display_name: "GitHub",
			},
			{
				id: "gitlab",
				type: "gitlab",
				authenticated: false,
				authenticate_url: "",
				display_icon: "/icon/gitlab.svg",
				display_name: "GitLab",
				optional: true,
			},
		],
	},
};

export const ExternalAuthAllConnected: Story = {
	args: {
		externalAuth: [
			{
				id: "github",
				type: "github",
				authenticated: true,
				authenticate_url: "",
				display_icon: "/icon/github.svg",
				display_name: "GitHub",
			},
			{
				id: "gitlab",
				type: "gitlab",
				authenticated: true,
				authenticate_url: "",
				display_icon: "/icon/gitlab.svg",
				display_name: "GitLab",
				optional: true,
			},
		],
	},
};

export const WithViewSourceButton: Story = {
	args: {
		canUpdateTemplate: true,
		versionId: "template-version-123",
		template: {
			...MockTemplate,
			organization_name: "default",
			name: "docker-template",
		},
	},
	parameters: {
		docs: {
			description: {
				story:
					"This story shows the View Source button that appears for template administrators. The button allows quick navigation to the template editor from the workspace creation page.",
			},
		},
	},
};

import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter3,
	MockUser,
	mockApiError,
} from "testHelpers/entities";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const meta: Meta<typeof CreateWorkspacePageView> = {
	title: "pages/CreateWorkspacePage",
	parameters: { chromatic },
	component: CreateWorkspacePageView,
	args: {
		defaultName: "",
		defaultOwner: MockUser,
		autofillParameters: [],
		template: MockTemplate,
		parameters: [],
		externalAuth: [],
		hasAllRequiredExternalAuth: true,
		mode: "form",
		permissions: {
			createWorkspaceForUser: true,
		},
		onCancel: action("onCancel"),
	},
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
				Parameters: [
					{
						Name: MockTemplateVersionParameter1.name,
						Value: "preset 1 override",
					},
				],
			},
			{
				ID: "preset-2",
				Name: "Preset 2",
				Parameters: [
					{
						Name: MockTemplateVersionParameter2.name,
						Value: "42",
					},
				],
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
		await userEvent.click(canvas.getByLabelText("Preset"));
		await userEvent.click(canvas.getByText("Preset 1"));
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

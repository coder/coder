import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	agentTemplates,
	richParametersKey,
	templateVersionPresetsKey,
} from "#/api/queries/templates";
import type { ChatProject, Preset, Template } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	mockApiError,
} from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

const dockerTemplate: Template = {
	...MockTemplate,
	id: "template-docker",
	name: "docker",
	display_name: "Docker",
	active_version_id: "version-docker",
};
const kubernetesTemplate: Template = {
	...MockTemplate,
	id: "template-kubernetes",
	name: "kubernetes",
	display_name: "Kubernetes",
	active_version_id: "version-kubernetes",
};
const smallPreset: Preset = {
	ID: "preset-small",
	Name: "Small",
	Parameters: [],
	Default: false,
	DesiredPrebuildInstances: null,
	Description: "",
	Icon: "",
};
const projectWithDefaultPreset: ChatProject = {
	...MockChatProject,
	templates: {
		template_ids: [dockerTemplate.id, kubernetesTemplate.id],
		default_template_id: dockerTemplate.id,
		default_preset_id: smallPreset.ID,
	},
};

const meta = {
	title: "pages/AgentsPage/ChatProjectDialog",
	component: ChatProjectDialog,
	args: {
		open: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		initialOrganizationId: MockDefaultOrganization.id,
		onOpenChange: fn(),
		isSubmitting: false,
		error: undefined,
		onSubmit: fn(),
	},
	parameters: {
		// Every story with an organization loads its templates.
		queries: [MockDefaultOrganization, MockOrganization2].map(
			(organization) => ({
				key: agentTemplates(organization.id).queryKey,
				data: [dockerTemplate, kubernetesTemplate],
			}),
		),
	},
} satisfies Meta<typeof ChatProjectDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const OpenOrganizationDropdown: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("combobox", { name: /Organization/ }),
		);
	},
};

export const SingleOrganization: Story = {
	args: {
		organizations: [MockOrganization2],
		initialOrganizationId: MockOrganization2.id,
	},
};

export const UnavailableOrganizations: Story = {
	args: {
		organizations: [],
		initialOrganizationId: undefined,
	},
};

export const UnavailableSelection: Story = {
	args: {
		organizations: [MockOrganization2],
	},
};

export const NameRequired: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByLabelText(/Project name/));
		await userEvent.tab();
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			"x".repeat(65),
		);
	},
};

export const NameFieldError: Story = {
	args: {
		project: MockChatProject,
		error: mockApiError({
			message: "Name must be at most 64 characters.",
			validations: [
				{ field: "name", detail: "Name must be at most 64 characters." },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByLabelText(/Name/));
		await userEvent.tab();
	},
};

export const Submitting: Story = {
	render: function Render(args) {
		const [isSubmitting, setIsSubmitting] = useState(false);
		return (
			<ChatProjectDialog
				{...args}
				isSubmitting={isSubmitting}
				onSubmit={() => setIsSubmitting(true)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			MockChatProject.name,
		);
		await userEvent.click(body.getByRole("button", { name: "Create project" }));
	},
};

export const MutationError: Story = {
	args: {
		error: new Error(
			"You do not have permission to create a project in this organization.",
		),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			MockChatProject.name,
		);
		await userEvent.type(
			body.getByRole("textbox", { name: "Description" }),
			MockChatProject.description,
		);
	},
};

export const Edit: Story = {
	args: {
		project: MockChatProject,
	},
};

export const Narrow: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const NarrowOpenOrganizationDropdown: Story = {
	...OpenOrganizationDropdown,
	globals: Narrow.globals,
};

export const EditDefaultPreset: Story = {
	args: { project: projectWithDefaultPreset },
	parameters: {
		queries: [
			{
				key: agentTemplates(MockChatProject.organization_id).queryKey,
				data: [dockerTemplate, kubernetesTemplate],
			},
			{
				key: templateVersionPresetsKey(dockerTemplate.active_version_id),
				data: [smallPreset],
			},
		],
	},
};

export const EditDefaultParameters: Story = {
	args: {
		project: {
			...projectWithDefaultPreset,
			templates: {
				template_ids: [dockerTemplate.id, kubernetesTemplate.id],
				default_template_id: dockerTemplate.id,
				default_parameters: { [MockTemplateVersionParameter1.name]: "custom" },
			},
		},
	},
	parameters: {
		queries: [
			{
				key: agentTemplates(MockChatProject.organization_id).queryKey,
				data: [dockerTemplate, kubernetesTemplate],
			},
			{
				key: richParametersKey(dockerTemplate.active_version_id),
				data: [MockTemplateVersionParameter1, MockTemplateVersionParameter2],
			},
		],
	},
};

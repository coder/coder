import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import {
	agentTemplates,
	richParametersKey,
	templateVersionPresetsKey,
} from "#/api/queries/templates";
import type { ChatProject, Preset, Template } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectTemplatesDialog } from "./ProjectTemplatesDialog";

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
const projectWithDefaultParameters: ChatProject = {
	...MockChatProject,
	templates: {
		template_ids: [dockerTemplate.id, kubernetesTemplate.id],
		default_template_id: dockerTemplate.id,
		default_parameters: { [MockTemplateVersionParameter1.name]: "custom" },
	},
};

const meta: Meta<typeof ProjectTemplatesDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectTemplatesDialog",
	component: ProjectTemplatesDialog,
	args: {
		project: MockChatProject,
		open: true,
		onOpenChange: fn(),
		isSaving: false,
		error: undefined,
		onSave: fn(),
	},
	parameters: {
		queries: [
			{
				key: agentTemplates(MockChatProject.organization_id).queryKey,
				data: [dockerTemplate, kubernetesTemplate],
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectTemplatesDialog>;

export const NoTemplates: Story = {};

export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getTemplates").mockReturnValue(new Promise(() => {}));
	},
};

export const DefaultPreset: Story = {
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

export const DefaultParameters: Story = {
	args: { project: projectWithDefaultParameters },
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
			// Lets the story switch to a preset without a request.
			{
				key: templateVersionPresetsKey(dockerTemplate.active_version_id),
				data: [smallPreset],
			},
		],
	},
};

export const Saving: Story = {
	args: { ...DefaultPreset.args, isSaving: true },
	parameters: DefaultPreset.parameters,
};

export const SaveError: Story = {
	args: {
		project: projectWithDefaultParameters,
		error: mockApiError({
			message:
				'Parameter "cpu" has an invalid value: value 99 is more than the maximum 8',
			validations: [
				{
					field: "templates.default_parameters",
					detail:
						'Parameter "cpu" has an invalid value: value 99 is more than the maximum 8',
				},
			],
		}),
	},
	parameters: DefaultParameters.parameters,
};

export const RequestError: Story = {
	args: {
		project: projectWithDefaultPreset,
		error: mockApiError({ message: "Failed to update chat project." }),
	},
	parameters: DefaultPreset.parameters,
};

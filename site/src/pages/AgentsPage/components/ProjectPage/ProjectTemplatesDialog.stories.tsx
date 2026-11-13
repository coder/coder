import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { useSyncExternalStore } from "react";
import { useQuery } from "react-query";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectsKey } from "#/api/queries/chatProjects";
import {
	agentTemplates,
	agentTemplatesKey,
	richParameters,
	richParametersKey,
	templateVersionPresets,
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

const mockDockerTemplate: Template = {
	...MockTemplate,
	id: "template-docker",
	name: "docker",
	display_name: "Docker",
	active_version_id: "version-docker",
};
const mockKubernetesTemplate: Template = {
	...MockTemplate,
	id: "template-kubernetes",
	name: "kubernetes",
	display_name: "Kubernetes",
	active_version_id: "version-kubernetes",
};
const mockSmallPreset: Preset = {
	ID: "preset-small",
	Name: "Small",
	Parameters: [],
	Default: false,
	DesiredPrebuildInstances: null,
	Description: "",
	Icon: "",
};
const mockProjectWithDefaultPreset: ChatProject = {
	...MockChatProject,
	templates: {
		template_ids: [mockDockerTemplate.id, mockKubernetesTemplate.id],
		default_template_id: mockDockerTemplate.id,
		default_preset_id: mockSmallPreset.ID,
	},
};
const mockProjectWithDefaultParameters: ChatProject = {
	...MockChatProject,
	templates: {
		template_ids: [mockDockerTemplate.id, mockKubernetesTemplate.id],
		default_template_id: mockDockerTemplate.id,
		default_parameters: { [MockTemplateVersionParameter1.name]: "custom" },
	},
};

type StoryQuery = {
	readonly queryKey: readonly unknown[];
	readonly queryFn: () => Promise<unknown>;
};

const RefetchOnMount: React.FC<
	React.PropsWithChildren<{ query: StoryQuery }>
> = ({ query, children }) => {
	useQuery({ ...query, staleTime: 0 });
	return children;
};

/**
 * Refetches a seeded query when the story mounts, so a failing request
 * leaves the query with both its cached data and an error.
 */
const withRefetch =
	(query: StoryQuery): Decorator =>
	(Story) => (
		<RefetchOnMount query={query}>
			<Story />
		</RefetchOnMount>
	);

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
		onResetError: fn(),
	},
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectTemplatesDialog>;

export const NoTemplates: Story = {};

export const Loading: Story = {
	parameters: {
		queries: [{ key: chatProjectsKey, data: [MockChatProject] }],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockReturnValue(new Promise(() => {}));
	},
};

export const DefaultPreset: Story = {
	args: { project: mockProjectWithDefaultPreset },
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [mockSmallPreset],
			},
		],
	},
};

export const DefaultParameters: Story = {
	args: { project: mockProjectWithDefaultParameters },
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
			{
				key: richParametersKey(mockDockerTemplate.active_version_id),
				data: [MockTemplateVersionParameter1, MockTemplateVersionParameter2],
			},
			// Lets the story switch to a preset without a request.
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [mockSmallPreset],
			},
		],
	},
};

export const TemplatesRefetchError: Story = {
	...DefaultPreset,
	decorators: [withRefetch(agentTemplates(MockChatProject.organization_id))],
	beforeEach: () => {
		spyOn(API, "getTemplates").mockRejectedValue(
			mockApiError({ message: "Failed to load templates." }),
		);
	},
	play: async () => {
		await screen.findByText("Failed to load templates.");
	},
};

export const DefaultPresetRefetchError: Story = {
	...DefaultPreset,
	decorators: [
		withRefetch(templateVersionPresets(mockDockerTemplate.active_version_id)),
	],
	beforeEach: () => {
		spyOn(API, "getTemplateVersionPresets").mockRejectedValue(
			mockApiError({ message: "Failed to load presets." }),
		);
	},
	play: async () => {
		await screen.findByText("Failed to load presets.");
	},
};

export const DefaultParametersRefetchError: Story = {
	...DefaultParameters,
	decorators: [
		withRefetch(richParameters(mockDockerTemplate.active_version_id)),
	],
	beforeEach: () => {
		spyOn(API, "getTemplateVersionRichParameters").mockRejectedValue(
			mockApiError({ message: "Failed to load parameters." }),
		);
	},
	play: async () => {
		await screen.findByText("Failed to load parameters.");
	},
};

export const Saving: Story = {
	args: { ...DefaultPreset.args, isSaving: true },
	parameters: DefaultPreset.parameters,
};

export const SaveError: Story = {
	args: {
		project: mockProjectWithDefaultParameters,
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
		project: mockProjectWithDefaultPreset,
		error: mockApiError({ message: "Failed to update chat project." }),
	},
	parameters: DefaultPreset.parameters,
};

/**
 * Stands in for the server's copy of the project, so the Conflict story's
 * play function can save a change from "another user" while the dialog is
 * open.
 */
const serverProject = (() => {
	let value = mockProjectWithDefaultPreset;
	const listeners = new Set<() => void>();
	return {
		get: () => value,
		set: (next: ChatProject) => {
			value = next;
			for (const listener of listeners) {
				listener();
			}
		},
		subscribe: (listener: () => void) => {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
	};
})();

/** Another user saved different templates while this editor was open. */
export const Conflict: Story = {
	parameters: DefaultPreset.parameters,
	beforeEach: () => {
		serverProject.set(mockProjectWithDefaultPreset);
	},
	render: function ConflictStory(args) {
		const project = useSyncExternalStore(
			serverProject.subscribe,
			serverProject.get,
		);
		return <ProjectTemplatesDialog {...args} project={project} />;
	},
	play: async () => {
		// An untouched draft follows the saved templates, so edit it first.
		await userEvent.click(
			await screen.findByRole("button", { name: "Remove Kubernetes" }),
		);
		await userEvent.keyboard("{Escape}");
		serverProject.set({
			...mockProjectWithDefaultPreset,
			templates: { template_ids: [mockKubernetesTemplate.id] },
		});
	},
};

export const StaleDefaultPreset: Story = {
	args: {
		project: {
			...mockProjectWithDefaultPreset,
			templates: {
				...mockProjectWithDefaultPreset.templates,
				default_preset_id: "preset-from-old-version",
			},
		},
	},
	parameters: DefaultPreset.parameters,
};

export const IneligibleTemplate: Story = {
	args: {
		project: {
			...MockChatProject,
			templates: {
				template_ids: [mockDockerTemplate.id, "template-deprecated"],
			},
		},
	},
};

export const ProjectRefreshError: Story = {
	parameters: {
		queries: [
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockRejectedValue(
			mockApiError({ message: "Failed to load chat projects." }),
		);
	},
	play: async () => {
		await screen.findByRole("button", { name: "Retry" });
	},
};

export const IneligibleDefaultTemplate: Story = {
	args: {
		project: {
			...MockChatProject,
			templates: {
				template_ids: [mockDockerTemplate.id, "template-deprecated"],
				default_template_id: "template-deprecated",
			},
		},
	},
};

export const SavedEmptyParameter: Story = {
	args: {
		project: {
			...mockProjectWithDefaultParameters,
			templates: {
				...mockProjectWithDefaultParameters.templates,
				default_parameters: { [MockTemplateVersionParameter1.name]: "" },
			},
		},
	},
	parameters: DefaultParameters.parameters,
};

export const RemovedClassicParameter: Story = {
	args: {
		project: {
			...mockProjectWithDefaultParameters,
			templates: {
				...mockProjectWithDefaultParameters.templates,
				default_parameters: {
					[MockTemplateVersionParameter1.name]: "custom",
					removed_parameter: "gone",
				},
			},
		},
	},
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [
					{ ...mockDockerTemplate, use_classic_parameter_flow: true },
					mockKubernetesTemplate,
				],
			},
			{
				key: richParametersKey(mockDockerTemplate.active_version_id),
				data: [MockTemplateVersionParameter1, MockTemplateVersionParameter2],
			},
		],
	},
};

export const RefreshingProject: Story = {
	parameters: DefaultPreset.parameters,
	args: { project: mockProjectWithDefaultPreset },
	decorators: [
		withRefetch({
			queryKey: chatProjectsKey,
			queryFn: () => API.experimental.getChatProjects(),
		}),
	],
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const RetryInProgress: Story = {
	parameters: { queries: [{ key: chatProjectsKey, data: [MockChatProject] }] },
	beforeEach: () => {
		spyOn(API, "getTemplates")
			.mockRejectedValueOnce(
				mockApiError({ message: "Failed to load templates." }),
			)
			.mockReturnValue(new Promise(() => {}));
	},
	play: async () => {
		await userEvent.click(await screen.findByRole("button", { name: "Retry" }));
	},
};

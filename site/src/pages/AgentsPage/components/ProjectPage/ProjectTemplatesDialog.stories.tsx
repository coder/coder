import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { useSyncExternalStore } from "react";
import { useQuery } from "react-query";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectsKey } from "#/api/queries/chatProjects";
import {
	agentTemplates,
	agentTemplatesKey,
	dynamicParameters,
	dynamicParametersKey,
	templateVersionPresetsKey,
} from "#/api/queries/templates";
import type {
	ChatProject,
	DynamicParametersResponse,
	Preset,
	Template,
} from "#/api/typesGenerated";
import {
	MockChatProject,
	MockPreviewParameter1,
	MockPreviewParameter2,
	MockTemplate,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
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
	templates: [mockDockerTemplate, mockKubernetesTemplate],
	default_template: {
		template_id: mockDockerTemplate.id,
		preset_id: mockSmallPreset.ID,
	},
};
const mockDefaultParameters = { [MockPreviewParameter1.name]: "custom" };
const mockProjectWithDefaultParameters: ChatProject = {
	...MockChatProject,
	templates: [mockDockerTemplate, mockKubernetesTemplate],
	default_template: {
		template_id: mockDockerTemplate.id,
		parameters: mockDefaultParameters,
	},
};
const mockParameters: DynamicParametersResponse = {
	id: 0,
	diagnostics: [],
	parameters: [MockPreviewParameter1, MockPreviewParameter2],
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
	decorators: [withAuthProvider],
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
		// The default parameter values are evaluated as this user.
		user: MockUserOwner,
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

/** The limit is on with nothing attached, so the dialog warns about it. */
export const LimitWithoutTemplates: Story = {
	args: { project: { ...MockChatProject, limit_templates: true } },
};

/**
 * Templates the user cannot access are attached. Only their count shows, and
 * the limit does not warn about having no templates.
 */
export const HiddenTemplates: Story = {
	args: {
		project: {
			...MockChatProject,
			limit_templates: true,
			hidden_template_count: 2,
		},
	},
};

export const OneHiddenTemplate: Story = {
	args: { project: { ...MockChatProject, hidden_template_count: 1 } },
};

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
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					mockDefaultParameters,
				),
				data: mockParameters,
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

export const DefaultParametersRefetchError: Story = {
	args: { project: mockProjectWithDefaultParameters },
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					mockDefaultParameters,
				),
				data: mockParameters,
			},
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [mockSmallPreset],
			},
		],
	},
	decorators: [
		withRefetch(
			dynamicParameters(
				mockDockerTemplate.active_version_id,
				MockUserOwner.id,
				mockDefaultParameters,
			),
		),
	],
	beforeEach: () => {
		spyOn(API, "getTemplateVersionDynamicParameters").mockRejectedValue(
			mockApiError({ message: "Failed to load parameters." }),
		);
	},
	play: async () => {
		await screen.findByText("Failed to load parameters.");
	},
};

export const Saving: Story = {
	args: { project: mockProjectWithDefaultPreset, isSaving: true },
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

export const SaveError: Story = {
	args: {
		project: mockProjectWithDefaultParameters,
		error: mockApiError({
			message:
				'Parameter "cpu" has an invalid value: value 99 is more than the maximum 8',
			validations: [
				{
					field: "default_template.parameters",
					detail:
						'Parameter "cpu" has an invalid value: value 99 is more than the maximum 8',
				},
			],
		}),
	},
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					mockDefaultParameters,
				),
				data: mockParameters,
			},
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [mockSmallPreset],
			},
		],
	},
};

export const RequestError: Story = {
	args: {
		project: mockProjectWithDefaultPreset,
		error: mockApiError({ message: "Failed to update chat project." }),
	},
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
			templates: [mockKubernetesTemplate],
			default_template: undefined,
		});
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

export const SavedEmptyParameter: Story = {
	args: {
		project: {
			...mockProjectWithDefaultParameters,
			default_template: {
				template_id: mockDockerTemplate.id,
				parameters: { [MockPreviewParameter1.name]: "" },
			},
		},
	},
	parameters: {
		queries: [
			{ key: chatProjectsKey, data: [MockChatProject] },
			{
				key: agentTemplatesKey(MockChatProject.organization_id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					{ [MockPreviewParameter1.name]: "" },
				),
				data: mockParameters,
			},
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [mockSmallPreset],
			},
		],
	},
};

export const RefreshingProject: Story = {
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

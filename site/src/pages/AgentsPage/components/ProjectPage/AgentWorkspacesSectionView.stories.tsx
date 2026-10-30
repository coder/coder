import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { AgentWorkspacesSectionView } from "./AgentWorkspacesSection";

const meta: Meta<typeof AgentWorkspacesSectionView> = {
	title: "pages/AgentsPage/ProjectPage/AgentWorkspacesSectionView",
	component: AgentWorkspacesSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		templates: [
			{
				id: "security",
				label: "Security scanner",
				isSelected: true,
				isOnlyAllowed: false,
				restrictedParameterCount: 2,
			},
			{
				id: "docs",
				label: "Docs builder",
				isSelected: true,
				isOnlyAllowed: false,
				restrictedParameterCount: 0,
			},
			{
				id: "gpu",
				label: "GPU workstation",
				isSelected: false,
				isOnlyAllowed: false,
				restrictedParameterCount: 0,
			},
		],
		externalAuthProviders: [
			{ id: "github", label: "GitHub", isSelected: true },
		],
		templatesError: undefined,
		onRetryTemplates: fn(),
		externalAuthError: undefined,
		onRetryExternalAuth: fn(),
		canEdit: true,
		isSaving: false,
		saveError: undefined,
		workspaceToolsEnabled: true,
		onWorkspaceToolsChange: fn(),
		onToggleTemplate: fn(),
		onConfigureParameters: fn(),
		onToggleExternalAuthProvider: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentWorkspacesSectionView>;

export const Restricted: Story = {};

export const OnlyOneTemplate: Story = {
	args: {
		templates: [
			{
				id: "security",
				label: "Security scanner",
				isSelected: true,
				isOnlyAllowed: true,
				restrictedParameterCount: 1,
			},
		],
	},
};

export const Saving: Story = { args: { isSaving: true } };

export const ReadOnly: Story = { args: { canEdit: false } };

export const WorkspaceToolsOff: Story = {
	args: { workspaceToolsEnabled: false },
};

export const WorkspaceToolsOffReadOnly: Story = {
	args: { workspaceToolsEnabled: false, canEdit: false },
};

export const NothingAvailable: Story = {
	args: { templates: [], externalAuthProviders: [] },
};

export const LoadingTemplates: Story = { args: { templates: undefined } };

export const LoadingProviders: Story = {
	args: { externalAuthProviders: undefined },
};

export const TemplatesLoadError: Story = {
	args: {
		templates: undefined,
		templatesError: mockApiError({ message: "Failed to load templates." }),
	},
};

/** A failed refetch keeps the providers and shows the error above them. */
export const ProvidersRefetchError: Story = {
	args: {
		externalAuthError: mockApiError({
			message: "Failed to refresh providers.",
		}),
	},
};

export const SaveError: Story = {
	args: {
		saveError: mockApiError({ message: "Failed to save changes." }),
	},
};

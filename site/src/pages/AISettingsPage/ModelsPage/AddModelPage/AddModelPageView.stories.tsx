import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import {
	MockChatModelProvider,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockCopilotProviderState,
	MockOpenAIProviderState,
} from "../testFixtures";
import AddModelPageView from "./AddModelPageView";

const meta: Meta<typeof AddModelPageView> = {
	title: "pages/AISettingsPage/ModelsPage/AddModelPageView",
	component: AddModelPageView,
	decorators: [
		(Story) => (
			<OrganizationModelsContext.Provider
				value={{
					organization: MockDefaultOrganization,
					organizations: [MockDefaultOrganization],
					permissions: MockOrganizationPermissions,
					requestedOrganizationDenied: false,
				}}
			>
				<Story />
			</OrganizationModelsContext.Provider>
		),
		withToaster,
	],
	args: {
		isLoading: false,
		loadError: null,
		refetchError: null,
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		isSaving: false,
		onProviderChange: fn(),
		onCreateModel: fn(async () => undefined),
	},
};

export default meta;
type Story = StoryObj<typeof AddModelPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: /add an? OpenAI model/i }),
		).toBeInTheDocument();
	},
};

export const WebSearchDependentFields: Story = {
	args: { selectedProviderState: MockAnthropicProviderState },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /provider configuration/i }),
		);
		expect(canvas.queryByLabelText(/allowed domains/i)).not.toBeInTheDocument();
		const webSearchField = canvas.getByRole("radiogroup", {
			name: /web search enabled/i,
		});
		await userEvent.click(
			within(webSearchField).getByRole("radio", { name: /on/i }),
		);
		const allowed = await canvas.findByLabelText(/allowed domains/i);
		const blocked = await canvas.findByLabelText(/blocked domains/i);
		await userEvent.type(allowed, "example.com");
		expect(blocked).toBeDisabled();
	},
};

export const NoProviderConfigurationFields: Story = {
	args: {
		providerStates: [MockCopilotProviderState],
		selectedProviderState: MockCopilotProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("button", { name: /provider configuration/i }),
		).not.toBeInTheDocument();
	},
};

export const NullAvailabilityModelsUsesEmptyCatalog: Story = {
	args: {
		providerStates: deriveProviderStates(
			[],
			[MockChatModelProviderDescriptor],
			{
				providers: [{ ...MockChatModelProvider, models: null }],
				unsupported_providers: [],
			},
		),
		selectedProviderState:
			deriveProviderStates([], [MockChatModelProviderDescriptor], {
				providers: [{ ...MockChatModelProvider, models: null }],
				unsupported_providers: [],
			})[0] ?? null,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("heading", { name: /add an? OpenAI model/i }),
		).toBeVisible();
	},
};

export const ProviderNotFound: Story = {
	args: { selectedProviderState: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Provider not found")).toBeInTheDocument();
	},
};

export const LoadError: Story = {
	args: { loadError: new Error("Failed to load models") },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load models")).toBeVisible();
	},
};

export const RefetchError: Story = {
	args: { refetchError: new Error("Failed to refresh models") },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to refresh models")).toBeVisible();
		expect(
			canvas.getByRole("heading", { name: /add an? OpenAI model/i }),
		).toBeVisible();
	},
};

export const Loading: Story = {
	args: { isLoading: true },
};

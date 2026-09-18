import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { ChatModel } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
	mockGPT5,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

const withOrganizationModels = (Story: React.FC) => (
	<OrganizationModelsContext.Provider
		value={{
			organization: MockDefaultOrganization,
			accessibleOrganizations: [MockDefaultOrganization],
			permissions: MockOrganizationPermissions,
			requestedOrganizationDenied: false,
		}}
	>
		<Story />
	</OrganizationModelsContext.Provider>
);

const meta: Meta<typeof ModelForm> = {
	title: "pages/AISettingsPage/ModelsPage/ModelForm",
	component: ModelForm,
	decorators: [withToaster, withOrganizationModels],
	args: {
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/models/add" },
			routing: [
				{ path: "/ai/settings/models/add", useStoryElement: true },
				{ path: "/ai/settings/models", element: <div>Models</div> },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ModelForm>;

const openProviderConfig = (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	return userEvent.click(
		canvas.getByRole("button", { name: /provider configuration/i }),
	);
};

// String enums render as dropdowns, booleans as on/off/default switches, and
// every control shares the same column width with its label on top. OpenAI is
// the densest provider, so it exercises the mixed grid layout.
export const ProviderConfigOpenAI: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await openProviderConfig(canvasElement);

		// String enums are dropdowns.
		for (const name of [
			/reasoning summary/i,
			/text verbosity/i,
			/service tier/i,
		]) {
			await expect(canvas.getByRole("combobox", { name })).toBeInTheDocument();
		}

		// Booleans keep the on/off/default segmented switch.
		for (const name of [
			/parallel tool calls/i,
			/store/i,
			/web search enabled/i,
		]) {
			const group = canvas.getByRole("radiogroup", { name });
			await expect(group).toBeInTheDocument();
			for (const option of ["Off", "On", "Default"]) {
				await expect(
					within(group).getByRole("radio", { name: option }),
				).toBeInTheDocument();
			}
		}
	},
};

// Anthropic is boolean-heavy, so it exercises the stacked tri-state switches.
export const ProviderConfigAnthropic: Story = {
	args: {
		selectedProviderState: MockAnthropicProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await openProviderConfig(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /thinking display/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("radiogroup", { name: /send reasoning/i }),
		).toBeInTheDocument();
	},
};

// Enabling web search reveals the gated search_context_size dropdown and the
// full-width allowed_domains JSON field.
export const ProviderConfigOpenAIWebSearch: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await openProviderConfig(canvasElement);
		const webSearch = canvas.getByRole("radiogroup", {
			name: /web search enabled/i,
		});
		await userEvent.click(within(webSearch).getByRole("radio", { name: "On" }));
		await expect(
			canvas.getByRole("combobox", { name: /search context size/i }),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText(/allowed domains/i)).toBeInTheDocument();
	},
};

const mockProModel: ChatModel = {
	...mockGPT5,
	model: "gpt-5.6-sol-2026-08-01",
	model_config: {
		reasoning_effort: { default: "medium", max: "high" },
		provider_options: {
			openai: { reasoning_mode: "pro", service_tier: "priority" },
		},
	},
};

const selectOption = async (
	canvasElement: HTMLElement,
	name: RegExp,
	option: string,
) => {
	await userEvent.click(within(canvasElement).getByRole("combobox", { name }));
	await userEvent.click(await screen.findByRole("option", { name: option }));
	await waitFor(() =>
		expect(screen.queryByRole("listbox")).not.toBeInTheDocument(),
	);
};

const expectEffortAndTier = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await expect(
		canvas.getByRole("combobox", { name: /default reasoning effort/i }),
	).toHaveTextContent("Medium");
	await expect(
		canvas.getByRole("combobox", { name: /max reasoning effort/i }),
	).toHaveTextContent("High");
	await expect(
		canvas.getByRole("combobox", { name: /service tier/i }),
	).toHaveTextContent("Priority");
};

export const AddProReasoningMode: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/model identifier/i), "gpt-5.6");
		await userEvent.click(
			await screen.findByRole("option", { name: /GPT-5.6 Sol/i }),
		);
		await waitFor(() =>
			expect(screen.queryByRole("listbox")).not.toBeInTheDocument(),
		);
		await userEvent.clear(canvas.getByLabelText(/context limit/i));
		await userEvent.type(canvas.getByLabelText(/context limit/i), "200000");
		await openProviderConfig(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /reasoning mode/i }),
		).toHaveTextContent("Default");
		await selectOption(canvasElement, /reasoning mode/i, "Pro");
		await selectOption(canvasElement, /default reasoning effort/i, "Medium");
		await selectOption(canvasElement, /max reasoning effort/i, "High");
		await selectOption(canvasElement, /service tier/i, "Priority");
		await expectEffortAndTier(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		await expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({
				model: "gpt-5.6-sol",
				model_config: {
					reasoning_effort: { default: "medium", max: "high" },
					provider_options: {
						openai: {
							reasoning_mode: "pro",
							service_tier: "priority",
							max_completion_tokens: 128000,
						},
					},
				},
			}),
		);
	},
};

export const EditAndClearReasoningMode: Story = {
	args: { editingModel: mockProModel },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await openProviderConfig(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /reasoning mode/i }),
		).toHaveTextContent("Pro");
		await selectOption(canvasElement, /reasoning mode/i, "Standard");
		await expectEffortAndTier(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /update model/i }),
		);
		await expect(args.onUpdateModel).toHaveBeenLastCalledWith(
			mockProModel.id,
			expect.objectContaining({
				model_config: {
					reasoning_effort: { default: "medium", max: "high" },
					provider_options: {
						openai: { reasoning_mode: "standard", service_tier: "priority" },
					},
				},
			}),
		);
		await selectOption(canvasElement, /reasoning mode/i, "Default");
		await expectEffortAndTier(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /update model/i }),
		);
		await expect(args.onUpdateModel).toHaveBeenLastCalledWith(
			mockProModel.id,
			expect.objectContaining({
				model_config: {
					reasoning_effort: { default: "medium", max: "high" },
					provider_options: { openai: { service_tier: "priority" } },
				},
			}),
		);
	},
};

export const ReasoningModeFiltersUnsupportedModel: Story = {
	args: { editingModel: mockProModel },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await openProviderConfig(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /reasoning mode/i }),
		).toHaveTextContent("Pro");
		const modelInput = canvas.getByLabelText(/model identifier/i);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "gpt-5.6-pro");
		await userEvent.tab();
		await expect(
			canvas.queryByRole("combobox", { name: /reasoning mode/i }),
		).not.toBeInTheDocument();
		await expectEffortAndTier(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /update model/i }),
		);
		await expect(args.onUpdateModel).toHaveBeenCalledWith(
			mockProModel.id,
			expect.objectContaining({
				model: "gpt-5.6-pro",
				model_config: {
					reasoning_effort: { default: "medium", max: "high" },
					provider_options: { openai: { service_tier: "priority" } },
				},
			}),
		);
	},
};

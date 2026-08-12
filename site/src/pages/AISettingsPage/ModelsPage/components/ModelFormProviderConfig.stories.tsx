import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

const meta: Meta<typeof ModelForm> = {
	title: "pages/AISettingsPage/ModelsPage/ModelForm",
	component: ModelForm,
	decorators: [withToaster],
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

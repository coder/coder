import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockOpenAICompatProviderState,
	MockOpenAIProviderState,
	mockCustomModel,
	mockCustomModelPricing,
	mockGPT5,
	mockGPT5Pricing,
	mockRefetchedCustomModelPricing,
} from "../testFixtures";
import UpdateModelPageView from "./UpdateModelPageView";

const meta: Meta<typeof UpdateModelPageView> = {
	title: "pages/AISettingsPage/ModelsPage/UpdateModelPageView",
	component: UpdateModelPageView,
	decorators: [withToaster],
	args: {
		model: mockGPT5,
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		modelPricing: mockGPT5Pricing,
		pricingProvider: "openai",
		isPricingLoading: false,
		isPricingFetching: false,
		pricingError: undefined,
		isPricingSaving: false,
		pricingSaveError: undefined,
		isPricingFeatureAvailable: true,
		canViewPricing: true,
		canEditPricing: true,
		onSavePricing: fn(async () => undefined),
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		onUpdateModel: fn(async () => undefined),
		onDeleteModel: fn(async () => undefined),
		onDuplicate: fn(),
		onToggleEnabled: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UpdateModelPageView>;

type PricingRefetchHarnessProps = ComponentProps<typeof UpdateModelPageView>;

const PricingRefetchHarness = (args: PricingRefetchHarnessProps) => {
	const [pricing, setPricing] = useState(args.modelPricing);
	return (
		<>
			<button
				type="button"
				onClick={() => setPricing(mockRefetchedCustomModelPricing)}
			>
				Apply server pricing
			</button>
			<UpdateModelPageView {...args} modelPricing={pricing} />
		</>
	);
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /^update model$/i }),
		).toBeVisible();
		await expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
		await expect(
			canvas.getByRole("heading", { name: /model pricing/i }),
		).toBeVisible();
	},
};

export const PriceBookModelIsReadOnly: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/managed by coder's price book/i),
		).toBeVisible();
		await expect(canvas.getByLabelText(/input tokens/i)).toHaveValue("1.25");
		await expect(canvas.getByLabelText(/output tokens/i)).toHaveValue("10");
		await expect(canvas.getByLabelText(/cache read tokens/i)).toHaveValue(
			"0.125",
		);
		await expect(canvas.getByLabelText(/cache write tokens/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/input tokens/i)).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /save pricing/i }),
		).not.toBeInTheDocument();
		await expect(args.onSavePricing).not.toHaveBeenCalled();
	},
};

export const CustomModelPricingIsEditable: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const inputPrice = canvas.getByLabelText(/input tokens/i);
		const outputPrice = canvas.getByLabelText(/output tokens/i);
		const cacheReadPrice = canvas.getByLabelText(/cache read tokens/i);
		await expect(inputPrice).toHaveValue("2.5");
		await expect(outputPrice).toHaveValue("8");
		await expect(cacheReadPrice).toHaveValue("");
		await expect(inputPrice).toBeEnabled();

		await userEvent.clear(inputPrice);
		await userEvent.type(inputPrice, "0.0079");
		await userEvent.clear(outputPrice);
		await userEvent.type(outputPrice, "12");
		await userEvent.click(
			canvas.getByRole("button", { name: /save pricing/i }),
		);

		await expect(args.onSavePricing).toHaveBeenCalledWith({
			provider: "openai",
			model: "custom-model",
			input_price: 7_900,
			output_price: 12_000_000,
			cache_read_price: null,
			cache_write_price: 3_125_000,
		});
		await expect(cacheReadPrice).toHaveValue("");
	},
};

export const UnknownPricingRequiresOneKnownPrice: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: undefined,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/no stored pricing/i)).toBeVisible();
		await expect(canvas.getByLabelText(/input tokens/i)).toHaveValue("");
		await userEvent.type(canvas.getByLabelText(/input tokens/i), "0");
		await userEvent.click(
			canvas.getByRole("button", { name: /save pricing/i }),
		);
		await expect(args.onSavePricing).toHaveBeenCalledWith({
			provider: "openai",
			model: "custom-model",
			input_price: 0,
			output_price: null,
			cache_read_price: null,
			cache_write_price: null,
		});
	},
};

export const InvalidPricingShowsInlineError: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: undefined,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/input tokens/i), "1.0000001");
		await userEvent.click(
			canvas.getByRole("button", { name: /save pricing/i }),
		);
		await expect(canvas.getByText(/up to 6 decimal places/i)).toBeVisible();
		await expect(args.onSavePricing).not.toHaveBeenCalled();
	},
};

export const PricingLoading: Story = {
	args: {
		modelPricing: undefined,
		isPricingLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/loading model pricing/i)).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: /save pricing/i }),
		).not.toBeInTheDocument();
	},
};

export const PricingLoadError: Story = {
	args: {
		modelPricing: undefined,
		pricingError: new Error("Unable to load model prices."),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Unable to load model prices.",
		);
		await expect(
			canvas.queryByLabelText(/input tokens/i),
		).not.toBeInTheDocument();
	},
};

export const PricingBackgroundRefetchErrorKeepsDraft: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
		pricingError: new Error("Unable to refresh model prices."),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const inputPrice = canvas.getByLabelText(/input tokens/i);
		await userEvent.clear(inputPrice);
		await userEvent.type(inputPrice, "4.25");
		await expect(inputPrice).toHaveValue("4.25");
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Unable to refresh model prices.",
		);
		await expect(inputPrice).toBeEnabled();
	},
};

export const PricingRefetchKeepsDraft: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
		isPricingFetching: true,
	},
	render: (args) => <PricingRefetchHarness {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const inputPrice = canvas.getByLabelText(/input tokens/i);
		await userEvent.clear(inputPrice);
		await userEvent.type(inputPrice, "4.25");
		await userEvent.click(
			canvas.getByRole("button", { name: /apply server pricing/i }),
		);
		await expect(inputPrice).toHaveValue("4.25");
		await expect(canvas.getByRole("status", { name: "" })).toHaveTextContent(
			/refreshing model pricing/i,
		);
	},
};

export const PricingSaveAcceptsLaterRefetch: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
	},
	render: (args) => <PricingRefetchHarness {...args} />,
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const inputPrice = canvas.getByLabelText(/input tokens/i);
		await userEvent.clear(inputPrice);
		await userEvent.type(inputPrice, "4.75");
		await userEvent.click(
			canvas.getByRole("button", { name: /save pricing/i }),
		);
		await expect(args.onSavePricing).toHaveBeenCalled();
		await expect(inputPrice).toHaveValue("4.75");
		await userEvent.click(
			canvas.getByRole("button", { name: /apply server pricing/i }),
		);
		await expect(inputPrice).toHaveValue("5.5");
	},
};

export const PricingPermissionIsReadOnly: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
		canEditPricing: false,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/read-only pricing/i)).toBeVisible();
		await expect(canvas.getByLabelText(/input tokens/i)).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /save pricing/i }),
		).not.toBeInTheDocument();
		await expect(args.onSavePricing).not.toHaveBeenCalled();
	},
};

export const PricingFeatureUnavailable: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: undefined,
		isPricingFeatureAvailable: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/ai bridge required/i)).toBeVisible();
		await expect(
			canvas.queryByLabelText(/input tokens/i),
		).not.toBeInTheDocument();
	},
};

export const OpenAICompatiblePricingUnavailable: Story = {
	args: {
		model: {
			...mockCustomModel,
			ai_provider_id: "prov-openai-compat",
		},
		providerStates: [MockOpenAICompatProviderState],
		selectedProviderState: MockOpenAICompatProviderState,
		modelPricing: undefined,
		pricingProvider: "openai-compat",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/not supported for openai-compatible providers/i),
		).toBeVisible();
		await expect(
			canvas.queryByLabelText(/input tokens/i),
		).not.toBeInTheDocument();
	},
};

export const PricingIdentityChangeDisablesEditing: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: mockCustomModelPricing,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.clear(canvas.getByLabelText(/model identifier/i));
		await userEvent.type(
			canvas.getByLabelText(/model identifier/i),
			"renamed-model",
		);
		await expect(
			canvas.getByText(/save model identity changes/i),
		).toBeVisible();
		await expect(canvas.getByLabelText(/input tokens/i)).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /save pricing/i }),
		).not.toBeInTheDocument();
		await expect(args.onSavePricing).not.toHaveBeenCalled();
	},
};

export const PricingViewPermissionMissing: Story = {
	args: {
		model: mockCustomModel,
		modelPricing: undefined,
		canViewPricing: false,
		canEditPricing: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/pricing unavailable/i)).toBeVisible();
		await expect(
			canvas.queryByLabelText(/input tokens/i),
		).not.toBeInTheDocument();
	},
};

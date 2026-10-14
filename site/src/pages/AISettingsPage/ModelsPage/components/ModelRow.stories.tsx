import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import {
	MockAnthropicProviderState,
	MockOpenAICompatProviderState,
	MockOpenAIProviderState,
	mockClaude,
	mockGPT5,
	mockOpenAICompatModel,
	mockOrphanedModel,
} from "../testFixtures";
import { ModelRow } from "./ModelRow";

const meta: Meta<typeof ModelRow> = {
	title: "pages/AISettingsPage/ModelsPage/ModelRow",
	component: ModelRow,
	args: {
		model: mockGPT5,
		provider: MockOpenAIProviderState,
		onClick: () => {},
	},
	render: (args) => (
		<Table>
			<TableBody>
				<ModelRow {...args} />
			</TableBody>
		</Table>
	),
};

export default meta;
type Story = StoryObj<typeof ModelRow>;

// Control case: a healthy, enabled model renders no status badge at all.
export const WithProvider: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("OpenAI")).toBeInTheDocument();
		expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();
		expect(canvas.queryByText("Unavailable")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};

const MockOpenAICompatProviderWithCustomIcon = {
	...MockOpenAICompatProviderState,
	providerDescriptor: {
		...MockOpenAICompatProviderState.providerDescriptor,
		icon: "/icon/coder.svg",
	},
};

export const CustomIconWithUnmappedProviderType: Story = {
	args: {
		model: mockOpenAICompatModel,
		provider: MockOpenAICompatProviderWithCustomIcon,
	},
};

// An unmapped provider type falls back to the generic icon, so the icon's
// tooltip is what tells it apart from other providers.
export const UnmappedProviderType: Story = {
	args: {
		model: mockOpenAICompatModel,
		provider: MockOpenAICompatProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(
			canvas.getByRole("img", { name: "Provider type: openai-compat" }),
		);
		await within(document.body).findByRole("tooltip");
	},
};

// A missing (soft-deleted) provider shows "Unset" plus the "Unavailable"
// notice even though the persisted model.enabled flag is true.
export const WithoutProviderForcesDisabled: Story = {
	args: {
		model: mockOrphanedModel,
		provider: undefined,
		onClick: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Unset")).toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();

		// The badge is keyboard-focusable and must open its tooltip without
		// activating the clickable row.
		const notice = canvas.getByRole("button", { name: "Unavailable" });
		notice.focus();
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(
			"The provider connected to this model has been deleted.",
		);
		await userEvent.keyboard("{Enter}");
		expect(args.onClick).not.toHaveBeenCalled();
	},
};

const MockDisabledAnthropicProvider = {
	...MockAnthropicProviderState,
	providerDescriptor: {
		...MockAnthropicProviderState.providerDescriptor,
		enabled: false,
	},
};

// A disabled provider keeps its label but the model shows the "Unavailable"
// notice because it is not usable.
export const DisabledProviderForcesDisabled: Story = {
	args: {
		model: { ...mockClaude, enabled: true, is_default: false },
		provider: MockDisabledAnthropicProvider,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		const notice = canvas.getByRole("button", { name: "Unavailable" });
		await expect(notice).toBeInTheDocument();
		expect(canvas.queryByText("Disabled")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();

		await userEvent.hover(notice);
		const tooltip = await within(document.body).findByRole("tooltip");
		await expect(tooltip).toHaveTextContent(
			"The provider connected to this model is disabled.",
		);
	},
};

// enabled=false with a healthy provider: "Disabled" badge beside the name,
// no "Unavailable" notice, and no "Unset" wording.
export const DisabledModelWithProvider: Story = {
	args: {
		model: { ...mockClaude, enabled: false, is_default: false },
		provider: MockAnthropicProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		const nameCell = canvas.getByRole("cell", { name: /Claude Sonnet 4.5/ });
		await expect(within(nameCell).getByText("Disabled")).toBeInTheDocument();
		expect(canvas.queryByText("Unavailable")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Unset")).not.toBeInTheDocument();
	},
};

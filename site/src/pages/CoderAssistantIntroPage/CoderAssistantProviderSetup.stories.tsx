import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { CoderAssistantProviderSetup } from "./CoderAssistantProviderSetup";

const meta: Meta<typeof CoderAssistantProviderSetup> = {
	title: "pages/CoderAssistantIntroPage/CoderAssistantProviderSetup",
	component: CoderAssistantProviderSetup,
	args: {
		onComplete: fn(),
		onSkip: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof CoderAssistantProviderSetup>;

export const Initial: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Select a provider above to continue"),
		).toBeInTheDocument();
		expect(canvas.queryByLabelText("API Key")).toBeNull();
	},
};

export const ProviderSelected: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Anthropic" }));
		expect(canvas.getByLabelText("API Key")).toBeInTheDocument();
		expect(canvas.getByLabelText("Base URL (optional)")).toHaveValue(
			"https://api.anthropic.com",
		);
		expect(canvas.getAllByRole("radio").length).toBeGreaterThan(0);
		expect(
			canvas.getByRole("button", { name: "Save & Continue" }),
		).toBeDisabled();
	},
};

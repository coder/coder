import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { ContextWindowSettings } from "./ContextWindowSettings";

const meta: Meta<typeof ContextWindowSettings> = {
	title: "pages/AgentsPage/ContextWindowSettings",
	component: ContextWindowSettings,
	args: {
		contextWindowConfigData: { fallback: "summary" },
		isLoadingContextWindowConfig: false,
		onSaveContextWindowConfig: fn(),
		isSavingContextWindowConfig: false,
		contextWindowConfigSaveError: null,
	},
};
export default meta;
type Story = StoryObj<typeof ContextWindowSettings>;

export const Default: Story = {};

export const NewWindow: Story = {
	args: { contextWindowConfigData: { fallback: "new_window" } },
};

export const Loading: Story = {
	args: {
		contextWindowConfigData: undefined,
		isLoadingContextWindowConfig: true,
	},
};

export const SelectNewWindow: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Context window fallback" }),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Start a new context window" }),
		);
		const save = await canvas.findByRole("button", { name: "Save" });
		await waitFor(() => expect(save).toBeEnabled());
		await userEvent.click(save);
		await waitFor(() =>
			expect(args.onSaveContextWindowConfig).toHaveBeenCalledWith(
				{ fallback: "new_window" },
				expect.anything(),
			),
		);
	},
};

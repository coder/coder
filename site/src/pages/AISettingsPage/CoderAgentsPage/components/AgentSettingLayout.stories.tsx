import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { AgentSettingLayout } from "./AgentSettingLayout";

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout",
	component: AgentSettingLayout,
	args: {
		title: "General model",
		description:
			"Used by delegated agents that can edit files or run commands.",
		showSave: false,
		isSaving: false,
		isSavedVisible: false,
		saveDisabled: true,
		onSubmit: fn(),
	},
} satisfies Meta<typeof AgentSettingLayout>;

export default meta;
type Story = StoryObj<typeof AgentSettingLayout>;

export const Default: Story = {
	args: {
		children: <Button type="button">Choose model</Button>,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("form", { name: "General model" }),
		).toBeVisible();
		expect(canvas.getByText("General model")).toBeVisible();
		expect(canvas.getByRole("button", { name: "Choose model" })).toBeVisible();
	},
};

export const Saving: Story = {
	args: {
		showSave: true,
		isSaving: true,
		saveDisabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByRole("button", { name: /save/i })).toBeDisabled();
	},
};

export const Saved: Story = {
	args: {
		showSave: false,
		isSavedVisible: true,
		saveDisabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Saved")).toBeVisible();
	},
};

export const WithError: Story = {
	args: {
		error: <p className="m-0">Failed to save setting.</p>,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to save setting.")).toBeVisible();
	},
};

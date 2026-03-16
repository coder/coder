import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import { FilterPresetMenu } from "./Filter";

const presets = [
	{ name: "My workspaces", query: "owner:me" },
	{ name: "All workspaces", query: "" },
	{ name: "Running", query: "status:running" },
	{ name: "Failed", query: "status:failed" },
];

const meta: Meta<typeof FilterPresetMenu> = {
	title: "components/Filter/FilterPresetMenu",
	component: FilterPresetMenu,
	args: {
		value: "",
		presets,
		onSelect: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof FilterPresetMenu>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /filters/i }));
	},
};

export const ActivePreset: Story = {
	args: {
		value: "status:running",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /filters/i }));
	},
};

export const WithLearnMoreLink: Story = {
	args: {
		learnMoreLink: "https://coder.com/docs",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /filters/i }));
	},
};

export const WithTwoLearnMoreLinks: Story = {
	args: {
		learnMoreLink: "https://coder.com/docs",
		learnMoreLabel2: "User status",
		learnMoreLink2: "https://coder.com/docs/users#status",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /filters/i }));
	},
};

export const SelectPreset: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /filters/i }));
		const item = screen.getByRole("menuitemradio", { name: "Running" });
		await expect(item).toBeVisible();
		await userEvent.click(item);
	},
};

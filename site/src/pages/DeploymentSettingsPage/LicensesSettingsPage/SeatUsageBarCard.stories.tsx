import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { SeatUsageBarCard } from "./SeatUsageBarCard";

const meta: Meta<typeof SeatUsageBarCard> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPage/SeatUsageBarCard",
	component: SeatUsageBarCard,
	args: {
		title: "Seat usage",
		actual: 1923,
		limit: 2500,
	},
};

export default meta;
type Story = StoryObj<typeof SeatUsageBarCard>;

export const Default: Story = {};

export const NearLimit: Story = {
	args: {
		actual: 2400,
		limit: 2500,
	},
};

export const OverLimit: Story = {
	args: {
		actual: 2600,
		limit: 2500,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("2,600")).toBeInTheDocument();
		await expect(canvas.getByText("2,500")).toBeInTheDocument();
	},
};

export const MissingActual: Story = {
	args: {
		actual: undefined,
		limit: 1000,
	},
};

export const ErrorInvalidLimit: Story = {
	args: {
		actual: 100,
		limit: undefined,
	},
};

export const Unlimited: Story = {
	args: {
		actual: 1923,
		limit: undefined,
		allowUnlimited: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,923")).toBeInTheDocument();
		await expect(canvas.getByText("Unlimited")).toBeInTheDocument();
	},
};

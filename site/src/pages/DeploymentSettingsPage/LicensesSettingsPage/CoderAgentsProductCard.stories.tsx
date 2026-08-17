import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { CoderAgentsProductCard } from "./CoderAgentsProductCard";

const meta: Meta<typeof CoderAgentsProductCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/CoderAgentsProductCard",
	component: CoderAgentsProductCard,
	args: {
		allocation: 20000,
		// Fractional usage renders with one decimal; whole values render
		// with a trailing .0 (see Exceeded).
		actual: 16264.3,
		isExceeded: false,
		isHardLimitExceeded: false,
	},
};

export default meta;
type Story = StoryObj<typeof CoderAgentsProductCard>;

const getMetricValue = (canvas: ReturnType<typeof within>, label: string) =>
	canvas.getByText(label).parentElement?.nextElementSibling;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Coder Agents")).toBeInTheDocument();
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"16,264.3 / 20,000",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
		const manageUsage = canvas.getByRole("link", { name: "Manage usage" });
		await expect(manageUsage).toHaveAttribute("href", "/deployment/groups");
		const agentSettings = canvas.getByRole("link", { name: "Agent settings" });
		await expect(agentSettings).toHaveAttribute(
			"href",
			"/ai/settings/coder-agents",
		);
	},
};

export const TooltipInteractions: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		await step("open the Total Agent hours tooltip from keyboard", async () => {
			await userEvent.tab();
			await expect(
				canvas.getByRole("button", { name: "Total Agent hours information" }),
			).toHaveFocus();
			await waitFor(async () => {
				await expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Total agent runtime hours used out of the hours included in this license.",
				);
			});
			await userEvent.keyboard("{Escape}");
			await waitFor(async () => {
				await expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
			});
		});
		await step("open the Concurrent chats tooltip on hover", async () => {
			await userEvent.hover(
				canvas.getByRole("button", { name: "Concurrent chats information" }),
			);
			await waitFor(async () => {
				await expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Number of Coder Agents chats that can run at the same time.",
				);
			});
		});
	},
};

export const UnlimitedAllocation: Story = {
	args: {
		allocation: -1,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"Unlimited",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const NotProvidingUsage: Story = {
	args: {
		actual: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"\u2014 / 20,000",
		);
	},
};

export const Exceeded: Story = {
	args: {
		actual: 21000,
		isExceeded: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"21,000.0 / 20,000",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const HardLimitExceeded: Story = {
	args: {
		actual: 25000,
		isHardLimitExceeded: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"25,000.0 / 20,000",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"5",
		);
	},
};

export const NoAllocation: Story = {
	args: {
		allocation: undefined,
		actual: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Max concurrent chats"),
		).toHaveTextContent("5");
		await expect(
			canvas.queryByText(/Agent hours used/),
		).not.toBeInTheDocument();
		const upgrade = canvas.getByRole("link", { name: "Upgrade" });
		await expect(upgrade).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const NoAllocationWithUsage: Story = {
	args: {
		allocation: undefined,
		actual: 1234.5,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "Agent hours used")).toHaveTextContent(
			"1,234.5",
		);
		await expect(
			canvas.getByRole("link", { name: "Upgrade" }),
		).toBeInTheDocument();
	},
};

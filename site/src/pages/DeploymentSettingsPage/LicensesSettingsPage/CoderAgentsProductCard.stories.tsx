import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { CoderAgentsProductCard } from "./CoderAgentsProductCard";

const meta: Meta<typeof CoderAgentsProductCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/CoderAgentsProductCard",
	component: CoderAgentsProductCard,
	args: {
		allocation: 20000,
		actualMinutes: 975_858,
		isTrial: false,
		isSoftLimitReached: false,
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
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("975,858 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
		await expect(
			canvas.getByRole("link", { name: "View docs" }),
		).toHaveAttribute(
			"href",
			expect.stringContaining("/ai-coder/agents/licensing-usage"),
		);
	},
};

export const TooltipInteractions: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		await step(
			"open the Total Agent minutes tooltip from keyboard",
			async () => {
				await userEvent.tab();
				await expect(
					canvas.getByRole("button", {
						name: "Total Agent minutes information",
					}),
				).toHaveFocus();
				await waitFor(async () => {
					await expect(screen.getByRole("tooltip")).toHaveTextContent(
						"Total agent runtime minutes used out of the minutes included in this license.",
					);
				});
				await userEvent.keyboard("{Escape}");
				await waitFor(async () => {
					await expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
				});
			},
		);
		await step("open the Concurrent agents tooltip on hover", async () => {
			await userEvent.hover(
				canvas.getByRole("button", { name: "Concurrent agents information" }),
			);
			await waitFor(async () => {
				await expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Number of agents that can run at the same time.",
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
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("Unlimited");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const NotProvidingUsage: Story = {
	args: {
		actualMinutes: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("\u2014 / 1,200,000");
	},
};

export const SoftLimitReached: Story = {
	args: {
		actualMinutes: 975_858,
		isSoftLimitReached: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("975,858 / 1,200,000");
		await expect(canvas.getByRole("status")).toHaveTextContent(
			"Approaching minutes limit",
		);
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const Exceeded: Story = {
	args: {
		actualMinutes: 1_260_000,
		isExceeded: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,260,000 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
		await expect(canvas.queryByRole("status")).not.toBeInTheDocument();
	},
};

export const HardLimitExceeded: Story = {
	args: {
		actualMinutes: 1_500_000,
		isHardLimitExceeded: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,500,000 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"5",
		);
		await expect(canvas.getByRole("status")).toHaveTextContent("Limit reached");
		await userEvent.hover(
			canvas.getByRole("button", { name: "Concurrent agents information" }),
		);
		await waitFor(async () => {
			await expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Number of agents that can run at the same time. You've reached your limit: concurrent chats are now capped at 5 (down from unlimited).",
			);
		});
	},
};

export const Community: Story = {
	args: {
		allocation: undefined,
		actualMinutes: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Coder Agents")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Max concurrent agents"),
		).toHaveTextContent("5");
		await expect(
			getMetricValue(canvas, "Agent minutes used"),
		).toHaveTextContent("\u2014");
		await expect(
			canvas.getByRole("link", { name: "Try unlimited for 30 days" }),
		).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.getByRole("link", { name: "View docs" }),
		).toHaveAttribute(
			"href",
			expect.stringContaining("/ai-coder/agents/licensing-usage"),
		);
		await expect(
			canvas.queryByRole("link", { name: "Upgrade" }),
		).not.toBeInTheDocument();
	},
};

export const CommunityWithUsage: Story = {
	args: {
		allocation: undefined,
		actualMinutes: 74_070,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Agent minutes used"),
		).toHaveTextContent("74,070");
	},
};

export const Trial: Story = {
	args: {
		allocation: undefined,
		actualMinutes: 74_070,
		isTrial: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Coder Agents Trial")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Agent minutes used"),
		).toHaveTextContent("74,070");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
		await expect(canvas.getByRole("link", { name: "Upgrade" })).toHaveAttribute(
			"href",
			"mailto:sales@coder.com",
		);
		await expect(
			canvas.queryByRole("link", { name: "Try unlimited for 30 days" }),
		).not.toBeInTheDocument();
	},
};

export const TrialWithoutUsage: Story = {
	args: {
		allocation: undefined,
		actualMinutes: undefined,
		isTrial: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Agent minutes used"),
		).toHaveTextContent("\u2014");
	},
};

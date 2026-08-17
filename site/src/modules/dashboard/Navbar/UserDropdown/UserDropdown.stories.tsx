import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { meAISpendKey } from "#/api/queries/users";
import type { FeatureName, UserAISpendStatus } from "#/api/typesGenerated";
import { MockBuildInfo, MockUserOwner } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { UserDropdown } from "./UserDropdown";

const mockAISpend: UserAISpendStatus = {
	user_id: MockUserOwner.id,
	effective_group_id: "grp-789",
	effective_budget: {
		spend_limit_micros: 1_200_000_000,
		limit_source: "group",
	},
	current_spend_micros: 819_000_000,
	period_start: "2026-06-01T00:00:00Z",
	period_end: "2026-07-01T00:00:00Z",
};

const spendPeriodLabel = "Approximate AI spend June 1 - July 1, 2026";

const aiCostControl: { features: FeatureName[] } = {
	features: ["aibridge"],
};

const meta: Meta<typeof UserDropdown> = {
	title: "modules/dashboard/UserDropdown",
	component: UserDropdown,
	args: {
		user: MockUserOwner,
		buildInfo: MockBuildInfo,
		supportLinks: [
			{ icon: "docs", name: "Documentation", target: "" },
			{ icon: "bug", name: "Report a bug", target: "" },
			{ icon: "chat", name: "Join the Coder Discord", target: "" },
			{ icon: "star", name: "Star the Repo", target: "" },
			{ icon: "/icon/aws.svg", name: "Amazon Web Services", target: "" },
		],
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof UserDropdown>;

const openDropdown = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button"));
	return within(
		await within(canvasElement.ownerDocument.body).findByRole("menu"),
	);
};

const Example: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: mockAISpend }],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend without the aibridge feature", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText(/AI spend/i)).not.toBeInTheDocument();
		});
	},
};

export const WithAISpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: mockAISpend }],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows AI spend", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$819 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "68");
		});
	},
};

export const AISpendWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...mockAISpend, current_spend_micros: 1_080_000_000 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the warning marker near the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$1,080 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "90");
		});
	},
};

export const AISpendAtLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...mockAISpend, current_spend_micros: 1_200_000_000 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("marks spend at the limit as exceeded", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$1,200 / $1,200 USD"),
			);
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const AISpendExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...mockAISpend, current_spend_micros: 1_500_000_000 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the exceeded marker at the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$1,500 / $1,200 USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const AISpendUnlimited: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{ key: meAISpendKey, data: { ...mockAISpend, effective_budget: null } },
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows unlimited spend without a bar", async () => {
			await openDropdown(canvasElement);
			await waitFor(() => {
				expect(document.body).toHaveTextContent("$819 / Unlimited USD");
				expect(document.body).toHaveTextContent(spendPeriodLabel);
			});
			expect(
				screen.queryByRole("progressbar", { name: "AI spend usage" }),
			).not.toBeInTheDocument();
		});
	},
};

export const AISpendZeroSpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{ key: meAISpendKey, data: { ...mockAISpend, current_spend_micros: 0 } },
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows zero spend with an empty bar", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$0 / $1,200 USD"),
			);
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "0");
		});
	},
};

export const AISpendZeroLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: {
					...mockAISpend,
					current_spend_micros: 0,
					effective_budget: {
						spend_limit_micros: 0,
						limit_source: "group",
					},
				},
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows a zero limit without exceeding", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$0 / $0 USD"),
			);
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "0");
		});
	},
};

// Dropdown closed to isolate the avatar and its severity badge, which
// indicates AI spend limit severity.

export const AvatarBorderDisabled: Story = {
	parameters: {
		queries: [{ key: meAISpendKey, data: mockAISpend }],
	},
};

export const AvatarBorderNormal: Story = {
	parameters: {
		...aiCostControl,
		queries: [{ key: meAISpendKey, data: mockAISpend }],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows no severity indicator for normal spend", async () => {
			const canvas = within(canvasElement);
			expect(
				canvas.getByRole("button", { name: "User menu" }),
			).toBeInTheDocument();
		});
	},
};

export const AvatarBorderWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...mockAISpend, current_spend_micros: 1_080_000_000 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("labels the trigger with the warning state", async () => {
			const canvas = within(canvasElement);
			await canvas.findByRole("button", {
				name: "User menu. AI spend is nearing its limit",
			});
		});
	},
};

export const AvatarBorderExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: { ...mockAISpend, current_spend_micros: 1_500_000_000 },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("labels the trigger with the exceeded state", async () => {
			const canvas = within(canvasElement);
			await canvas.findByRole("button", {
				name: "User menu. AI spend limit exceeded",
			});
		});
	},
};

export const AISpendHiddenOnInvalidData: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{ key: meAISpendKey, data: { ...mockAISpend, current_spend_micros: -1 } },
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on invalid data", async () => {
			await openDropdown(canvasElement);
			expect(document.body).not.toHaveTextContent(spendPeriodLabel);
		});
	},
};

export const AISpendHiddenOnNegativeLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [
			{
				key: meAISpendKey,
				data: {
					...mockAISpend,
					effective_budget: { spend_limit_micros: -1, limit_source: "group" },
				},
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on a negative limit", async () => {
			await openDropdown(canvasElement);
			expect(document.body).not.toHaveTextContent(spendPeriodLabel);
		});
	},
};

export { Example as UserDropdown };

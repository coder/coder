import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { UserAISpend } from "#/api/api";
import { meAISpendKey } from "#/api/queries/users";
import type { Experiment, FeatureName } from "#/api/typesGenerated";
import { MockBuildInfo, MockUserOwner } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { UserDropdown } from "./UserDropdown";

function mockAISpend(overrides: Partial<UserAISpend> = {}): UserAISpend {
	return {
		user_id: MockUserOwner.id,
		spend_limit_micros: 1_200_000_000,
		effective_group_id: "grp-789",
		limit_source: "group",
		current_spend_micros: 819_000_000,
		period_start: "2026-06-01T00:00:00Z",
		period_end: "2026-07-01T00:00:00Z",
		...overrides,
	};
}

const aiSpendQuery = (overrides?: Partial<UserAISpend>) => ({
	key: meAISpendKey,
	data: mockAISpend(overrides),
});

const aiCostControl: { features: FeatureName[]; experiments: Experiment[] } = {
	features: ["aibridge"],
	experiments: ["ai-gateway-cost-control"],
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
	await waitFor(async () =>
		expect(await screen.findByText(/v2\.\d+\.\d+/i)).toBeInTheDocument(),
	);
};

const Example: Story = {
	parameters: {
		queries: [aiSpendQuery()],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend without cost control", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText(/AI spend/i)).not.toBeInTheDocument();
		});
	},
};

export const WithAISpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery()],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows AI spend", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$819 / $1,200 USD"),
			);
			expect(document.body).toHaveTextContent("(AI spend/month)");
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "68");
		});
	},
};

export const AISpendWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_080_000_000 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the warning marker near the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$1,080 / $1,200 USD"),
			);
			expect(document.body).toHaveTextContent("(AI spend/month)");
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "90");
		});
	},
};

export const AISpendAtLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_200_000_000 })],
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
		queries: [aiSpendQuery({ current_spend_micros: 1_500_000_000 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the exceeded marker at the limit", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$1,500 / $1,200 USD"),
			);
			expect(document.body).toHaveTextContent("(AI spend/month)");
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const AISpendUnlimited: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ spend_limit_micros: null })],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows unlimited spend without a bar", async () => {
			await openDropdown(canvasElement);
			await waitFor(() =>
				expect(document.body).toHaveTextContent("$819 / Unlimited USD"),
			);
			expect(document.body).toHaveTextContent("(AI spend/month)");
			expect(
				screen.queryByRole("progressbar", { name: "AI spend usage" }),
			).not.toBeInTheDocument();
		});
	},
};

export const AISpendZeroSpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 0 })],
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
		queries: [aiSpendQuery({ current_spend_micros: 0, spend_limit_micros: 0 })],
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

// Dropdown closed to isolate the avatar border, which reflects spend severity.

export const AvatarBorderDisabled: Story = {
	parameters: {
		queries: [aiSpendQuery()],
	},
};

export const AvatarBorderNormal: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery()],
	},
};

export const AvatarBorderWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_080_000_000 })],
	},
};

export const AvatarBorderExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_500_000_000 })],
	},
};

export const AISpendHiddenOnInvalidData: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: -1 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on invalid data", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText("(AI spend/month)")).not.toBeInTheDocument();
		});
	},
};

export const AISpendHiddenOnNegativeLimit: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ spend_limit_micros: -1 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend on a negative limit", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText("(AI spend/month)")).not.toBeInTheDocument();
		});
	},
};

export { Example as UserDropdown };

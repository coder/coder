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
		...overrides,
	};
}

const aiSpendQuery = (overrides?: Partial<UserAISpend>) => ({
	key: meAISpendKey,
	data: mockAISpend(overrides),
});

// Gates the AI spend section, matching the group budget UI.
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
		queries: [aiSpendQuery({ spend_limit_micros: null })],
	},
	play: async ({ canvasElement, step }) => {
		await step("click to open", async () => {
			await openDropdown(canvasElement);
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
			expect(
				await screen.findByText("AI spend - $819 / $1,200 USD"),
			).toBeInTheDocument();
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "68");
		});
	},
};

// 90% of the limit lands in the warning band (>=85%, <100%).
export const AISpendWarning: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_080_000_000 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the warning marker near the limit", async () => {
			await openDropdown(canvasElement);
			expect(
				await screen.findByText("AI spend - $1,080 / $1,200 USD"),
			).toBeInTheDocument();
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "90");
		});
	},
};

// Spend past the limit clamps the bar to 100% and marks it exceeded.
export const AISpendExceeded: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ current_spend_micros: 1_500_000_000 })],
	},
	play: async ({ canvasElement, step }) => {
		await step("shows the exceeded marker at the limit", async () => {
			await openDropdown(canvasElement);
			expect(
				await screen.findByText("AI spend - $1,500 / $1,200 USD"),
			).toBeInTheDocument();
			expect(
				screen.getByRole("progressbar", { name: "AI spend usage" }),
			).toHaveAttribute("aria-valuenow", "100");
		});
	},
};

export const WithoutAISpend: Story = {
	parameters: {
		...aiCostControl,
		queries: [aiSpendQuery({ spend_limit_micros: null })],
	},
	play: async ({ canvasElement, step }) => {
		await step("hides AI spend without a budget", async () => {
			await openDropdown(canvasElement);
			expect(screen.queryByText(/AI spend/i)).not.toBeInTheDocument();
		});
	},
};

export { Example as UserDropdown };

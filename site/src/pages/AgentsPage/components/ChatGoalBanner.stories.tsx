import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatGoalBanner } from "./ChatGoalBanner";

const storyNow = new Date().toISOString();

const longGoalObjective =
	"Ensure coder/coder has no frontend components using MUI, migrate remaining components to shared primitives, and leave tests and stories covering the replacement.";

const goal = (
	overrides: Partial<TypesGen.ChatGoal> = {},
): TypesGen.ChatGoal => ({
	id: "goal-1",
	root_chat_id: "chat-1",
	objective: longGoalObjective,
	status: "active",
	continuation_count: 0,
	created_by_user_id: "user-1",
	completed_by_agent: false,
	created_at: storyNow,
	updated_at: storyNow,
	...overrides,
});

const meta: Meta<typeof ChatGoalBanner> = {
	title: "pages/AgentsPage/ChatGoalBanner",
	component: ChatGoalBanner,
	args: {
		goal: goal(),
		onAction: fn(),
		canMutateGoal: true,
	},
};

export default meta;
type Story = StoryObj<typeof ChatGoalBanner>;

export const ActivePursuing: Story = {
	args: {
		isChatWorking: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Current goal")).toBeVisible();
		expect(canvas.getByText("Pursuing goal")).toBeVisible();
		expect(canvas.getByText(longGoalObjective)).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Pause/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Complete/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));

		expect(args.onAction).toHaveBeenNthCalledWith(1, "pause");
		expect(args.onAction).toHaveBeenNthCalledWith(2, "complete");
		expect(args.onAction).toHaveBeenNthCalledWith(3, "clear");
	},
};

export const ActiveIdle: Story = {
	args: {
		isChatWorking: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal active")).toBeVisible();
		expect(canvas.queryByText("Pursuing goal")).toBeNull();
	},
};

export const ActiveAutoContinuing: Story = {
	args: {
		goal: goal({ continuation_count: 3 }),
		isChatWorking: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Pursuing goal")).toBeVisible();
		expect(canvas.getByText("Auto-continue 3/10")).toBeVisible();
	},
};

export const Paused: Story = {
	args: {
		goal: goal({ status: "paused", paused_reason: "user" }),
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal paused")).toBeVisible();
		expect(canvas.getByText("Paused by you")).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Resume/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));

		expect(args.onAction).toHaveBeenNthCalledWith(1, "resume");
		expect(args.onAction).toHaveBeenNthCalledWith(2, "clear");
	},
};

export const PausedAtTurnLimit: Story = {
	args: {
		goal: goal({
			status: "paused",
			paused_reason: "turn_limit",
			continuation_count: 10,
		}),
		onAction: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal paused")).toBeVisible();
		expect(canvas.getByText("Turn limit reached")).toBeVisible();
		expect(canvas.getByRole("button", { name: /Resume/i })).toBeEnabled();
	},
};

export const Blocked: Story = {
	args: {
		goal: goal({
			status: "blocked",
			blocked_reason:
				"The migration requires a decision on whether to keep the legacy theme package. Reply with the direction and resume the goal.",
		}),
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal blocked")).toBeVisible();
		expect(canvas.getByText(/Blocked: The migration requires/)).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Resume/i }));
		expect(args.onAction).toHaveBeenCalledWith("resume");
	},
};

export const PausedResumeUnavailable: Story = {
	args: {
		goal: goal({ status: "paused", paused_reason: "user" }),
		isChatWorking: true,
		actionUnavailableReasons: {
			resume: "The chat is busy. Resume becomes available when it is idle.",
		},
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const resume = canvas.getByRole("button", { name: /Resume/i });
		// The gated action stays keyboard-focusable with an accessible
		// description instead of a hard disable, and clicking it no-ops.
		expect(resume).not.toBeDisabled();
		expect(resume).toHaveAttribute("aria-disabled", "true");
		expect(resume).toHaveAccessibleDescription(
			"The chat is busy. Resume becomes available when it is idle.",
		);
		resume.focus();
		expect(resume).toHaveFocus();
		await userEvent.click(resume);
		expect(args.onAction).not.toHaveBeenCalledWith("resume");
		// Clear stays available while resume is gated.
		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));
		expect(args.onAction).toHaveBeenCalledWith("clear");
	},
};

export const Complete: Story = {
	args: {
		goal: goal({
			status: "complete",
			completion_summary: "Verified and shipped.",
		}),
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal complete")).toBeVisible();
		expect(canvas.getByText("Summary: Verified and shipped.")).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));

		expect(args.onAction).toHaveBeenCalledWith("clear");
	},
};

export const ReadOnlyChildGoal: Story = {
	args: {
		goal: goal(),
		canMutateGoal: false,
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Current goal")).toBeVisible();
		expect(canvas.getByText("Goal active")).toBeVisible();
		expect(canvas.queryByRole("button", { name: /Pause/i })).toBeNull();
		expect(args.onAction).not.toHaveBeenCalled();
	},
};

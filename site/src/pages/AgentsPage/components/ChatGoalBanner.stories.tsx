import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatGoalBanner } from "./ChatGoalBanner";

const goal = (
	overrides: Partial<TypesGen.ChatGoal> = {},
): TypesGen.ChatGoal => ({
	id: "goal-1",
	root_chat_id: "chat-1",
	objective: "Ship the frontend goal command UX with tests.",
	status: "active",
	created_by_user_id: "user-1",
	completed_by_agent: false,
	created_at: "2026-05-22T00:00:00Z",
	updated_at: "2026-05-22T00:00:00Z",
	...overrides,
});

const meta: Meta<typeof ChatGoalBanner> = {
	title: "pages/AgentsPage/ChatGoalBanner",
	component: ChatGoalBanner,
	args: {
		goal: goal(),
		onAction: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatGoalBanner>;

export const Active: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Current goal")).toBeVisible();
		expect(canvas.getByText("Active")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: /Pause/i }));
		expect(args.onAction).toHaveBeenCalledWith("pause");
	},
};

export const Paused: Story = {
	args: {
		goal: goal({ status: "paused" }),
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Paused")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: /Resume/i }));
		expect(args.onAction).toHaveBeenCalledWith("resume");
	},
};

export const Complete: Story = {
	args: {
		goal: goal({
			status: "complete",
			completion_summary: "All goal UX work is complete.",
		}),
		onAction: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Complete")).toBeVisible();
		expect(canvas.getByText(/All goal UX work is complete/)).toBeVisible();
		expect(canvas.getByRole("button", { name: /Clear/i })).toBeVisible();
	},
};

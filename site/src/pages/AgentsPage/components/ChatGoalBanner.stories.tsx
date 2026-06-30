import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatGoalBanner } from "./ChatGoalBanner";

const storyNow = new Date().toISOString();

const goal = (
	overrides: Partial<TypesGen.ChatGoal> = {},
): TypesGen.ChatGoal => ({
	id: "goal-1",
	root_chat_id: "chat-1",
	objective: "Ship the frontend goal mode UX with tests.",
	status: "active",
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

export const Active: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Current goal")).toBeVisible();
		expect(canvas.getByText("Pursuing goal")).toBeVisible();
		expect(
			canvas.getByText("Ship the frontend goal mode UX with tests."),
		).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Pause/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Complete/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));

		expect(args.onAction).toHaveBeenNthCalledWith(1, "pause");
		expect(args.onAction).toHaveBeenNthCalledWith(2, "complete");
		expect(args.onAction).toHaveBeenNthCalledWith(3, "clear");
	},
};

export const Paused: Story = {
	args: {
		goal: goal({ status: "paused" }),
		onAction: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Goal paused")).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: /Resume/i }));
		await userEvent.click(canvas.getByRole("button", { name: /Clear/i }));

		expect(args.onAction).toHaveBeenNthCalledWith(1, "resume");
		expect(args.onAction).toHaveBeenNthCalledWith(2, "clear");
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
		expect(canvas.getByText("Pursuing goal")).toBeVisible();
		expect(canvas.queryByRole("button", { name: /Pause/i })).toBeNull();
		expect(args.onAction).not.toHaveBeenCalled();
	},
};

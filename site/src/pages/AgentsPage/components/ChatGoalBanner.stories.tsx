import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatGoal } from "#/testHelpers/chatEntities";
import { ChatGoalBanner } from "./ChatGoalBanner";

const storyNow = new Date().toISOString();

const longGoalObjective =
	"Ensure coder/coder has no frontend components using MUI, migrate remaining components to shared primitives, and leave tests and stories covering the replacement.";

const goal = (
	overrides: Partial<TypesGen.ChatGoal> = {},
): TypesGen.ChatGoal => ({
	...MockChatGoal,
	objective: longGoalObjective,
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
};

export const ActiveIdle: Story = {
	args: {
		isChatWorking: false,
	},
};

export const Paused: Story = {
	args: {
		goal: goal({ status: "paused" }),
	},
};

export const PausedResumeUnavailable: Story = {
	args: {
		goal: goal({ status: "paused" }),
		isChatWorking: true,
		actionUnavailableReasons: {
			resume: "The chat is busy. Resume becomes available when it is idle.",
		},
	},
};

export const Complete: Story = {
	args: {
		goal: goal({
			status: "complete",
			completion_summary: "Verified and shipped.",
		}),
	},
};

export const ReadOnlyChildGoal: Story = {
	args: {
		canMutateGoal: false,
	},
};

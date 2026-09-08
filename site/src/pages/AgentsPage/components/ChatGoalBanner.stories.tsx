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

export const ActiveAutoContinuing: Story = {
	args: {
		goal: goal({ continuation_count: 3 }),
		isChatWorking: true,
	},
};

export const Paused: Story = {
	args: {
		goal: goal({ status: "paused", paused_reason: "user" }),
	},
};

export const PausedAtTurnLimit: Story = {
	args: {
		goal: goal({
			status: "paused",
			paused_reason: "turn_limit",
			continuation_count: 10,
		}),
	},
};

export const Blocked: Story = {
	args: {
		goal: goal({
			status: "blocked",
			blocked_reason:
				"The migration requires a decision on whether to keep the legacy theme package. Reply with the direction and resume the goal.",
		}),
	},
};

export const PausedResumeUnavailable: Story = {
	args: {
		goal: goal({ status: "paused", paused_reason: "user" }),
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

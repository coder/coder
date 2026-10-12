import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutomationLastRun } from "./AutomationLastRun";

const now = new Date("2026-10-12T12:00:00Z");
const at = "2026-10-12T11:55:00Z";

const meta: Meta<typeof AutomationLastRun> = {
	title: "pages/AgentsPage/Automations/AutomationLastRun",
	component: AutomationLastRun,
	args: { now, lastRun: null },
};

export default meta;
type Story = StoryObj<typeof AutomationLastRun>;

export const NoRuns: Story = {};

export const Accepted: Story = {
	args: { lastRun: { at, outcome: "accepted", chat_id: "chat-1" } },
};

export const Skipped: Story = {
	args: { lastRun: { at, outcome: "skipped", reason: "chat_busy" } },
};

export const RefusedWithHint: Story = {
	args: {
		lastRun: { at, outcome: "refused", reason: "hook_denied" },
		showHint: true,
	},
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { TriggerScheduleDialog } from "./TriggerScheduleDialog";

const meta: Meta<typeof TriggerScheduleDialog> = {
	title: "pages/AgentsPage/ProjectPage/TriggerScheduleDialog",
	component: TriggerScheduleDialog,
	args: {
		agentName: MockChatProject.name,
		isCreating: false,
		error: undefined,
		// Pinned so the next-run preview stays the same in screenshots.
		now: new Date("2026-09-04T12:00:00Z"),
		timeZone: "UTC",
		onScheduleChange: fn(),
		onClose: fn(),
		onCreate: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TriggerScheduleDialog>;

export const Default: Story = {};

export const Creating: Story = { args: { isCreating: true } };

export const ScheduleRejected: Story = {
	args: {
		error: mockApiError({
			message: "Invalid trigger.",
			validations: [
				{
					field: "schedule",
					detail:
						"Schedule runs every 2 minutes; the minimum interval is 5 minutes.",
				},
			],
		}),
	},
};

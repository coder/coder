import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentAlert, StartScriptFailureDetail } from "./AgentAlert";

const meta: Meta<typeof AgentAlert> = {
	title: "pages/WorkspacePage/AgentAlert",
	component: AgentAlert,
	args: {
		title: "Something went wrong",
		detail:
			"A useful description of what happened and what the user can do about it.",
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export default meta;
type Story = StoryObj<typeof AgentAlert>;

export const WarningProminent: Story = {
	args: {
		severity: "warning",
		prominent: true,
	},
};

export const WarningSubtle: Story = {
	args: {
		severity: "warning",
		prominent: false,
	},
};

export const InfoProminent: Story = {
	args: {
		severity: "info",
		prominent: true,
	},
};

export const InfoSubtle: Story = {
	args: {
		severity: "info",
		prominent: false,
	},
};

export const WithoutTroubleshootingURL: Story = {
	args: {
		severity: "warning",
		prominent: true,
		troubleshootingURL: undefined,
	},
};

export const WithScriptTimingDetail: Story = {
	render: (args) => (
		<AgentAlert
			{...args}
			detail={
				<StartScriptFailureDetail
					baseDetail="A startup script exited with an error. Check the agent logs for details."
					timings={[
						{
							display_name: "Startup Script",
							exit_code: 1,
							stage: "start",
							status: "exit_failure",
							started_at: "2021-05-05T00:00:00.000Z",
							ended_at: "2021-05-05T00:00:01.000Z",
							workspace_agent_id: "test-agent-id",
							workspace_agent_name: "test-agent",
						},
					]}
				/>
			}
		/>
	),
	args: {
		title: "Startup script failed",
		severity: "warning",
		prominent: false,
		troubleshootingURL: undefined,
	},
};

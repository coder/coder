import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	SCENARIO_AGENT_CRASH,
	SCENARIO_DERP_FALLBACK,
	SCENARIO_DEVICE_SLEEP,
	SCENARIO_WORKSPACE_STOP,
} from "./mockData";
import { OperatorDiagnosticPageView } from "./OperatorDiagnosticPageView";

const meta: Meta<typeof OperatorDiagnosticPageView> = {
	title: "pages/OperatorDiagnosticPage",
	component: OperatorDiagnosticPageView,
	args: {
		username: "sarah-chen",
		selectedHours: 72,
		onUserSelect: () => {},
		onTimeWindowChange: () => {},
		isDemo: true,
		statusFilter: "all",
		onStatusFilterChange: () => {},
		workspaceFilter: "all",
		onWorkspaceFilterChange: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OperatorDiagnosticPageView>;

export const DeviceSleep: Story = {
	args: {
		data: SCENARIO_DEVICE_SLEEP,
		isLoading: false,
		username: "sarah-chen",
	},
};

export const WorkspaceStop: Story = {
	args: {
		data: SCENARIO_WORKSPACE_STOP,
		isLoading: false,
		username: "john-ops",
	},
};

export const DERPFallback: Story = {
	args: {
		data: SCENARIO_DERP_FALLBACK,
		isLoading: false,
		username: "alex-dev",
	},
};

export const AgentCrash: Story = {
	args: {
		data: SCENARIO_AGENT_CRASH,
		isLoading: false,
		username: "priya-ml",
	},
};

export const Loading: Story = {
	args: {
		data: undefined,
		isLoading: true,
		username: "sarah-chen",
	},
};

export const Empty: Story = {
	args: {
		data: undefined,
		isLoading: false,
		username: "unknown-user",
	},
};

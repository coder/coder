import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToolCallTable } from "./ToolCallTable";

const meta: Meta<typeof ToolCallTable> = {
	title: "pages/AIBridgePage/SessionTimeline/ToolCallTable",
	component: ToolCallTable,
};

export default meta;
type Story = StoryObj<typeof ToolCallTable>;

export const WithMCPServer: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL: "http://localhost:3000/mcp",
	},
};

export const WithoutMCPServer: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL: "",
	},
};

export const LongServerURL: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL:
			"https://very-long-mcp-server-hostname.internal.example.com/api/v2/mcp/tools",
	},
};

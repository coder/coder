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
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const WithoutMCPServer: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL: "",
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const WithTokenMetadata: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL: "https://mcp.example.com/api/v1",
		inputTokens: 5000,
		outputTokens: 2500,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const LongServerURL: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		serverURL:
			"https://very-long-mcp-server-hostname.internal.example.com/api/v2/mcp/tools",
		inputTokens: 800,
		outputTokens: 200,
	},
};

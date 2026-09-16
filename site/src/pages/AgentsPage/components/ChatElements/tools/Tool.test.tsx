import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { McpAppPanelContext } from "../../McpApp/McpAppPanelContext";
import { Tool } from "./Tool";

const taskboardServer = {
	...MockMCPServerConfig,
	display_name: "Task board",
};

describe("Tool", () => {
	it("opens the MCP App bound to the tool call", async () => {
		const user = userEvent.setup();
		const openMcpApp = vi.fn();

		renderComponent(
			<McpAppPanelContext value={{ openMcpApp }}>
				<Tool
					name="add_task"
					status="completed"
					args={{ title: "Ship it" }}
					result="Added task #1"
					mcpServerConfigId={taskboardServer.id}
					mcpServers={[taskboardServer]}
					toolCallId="call-1"
					mcpAppResourceUri="ui://taskboard/board"
				/>
			</McpAppPanelContext>,
		);

		await user.click(
			screen.getByRole("button", { name: /Open Task board app/ }),
		);

		expect(openMcpApp).toHaveBeenCalledWith({
			mcpServerConfigId: taskboardServer.id,
			resourceUri: "ui://taskboard/board",
			toolCallId: "call-1",
			label: "Task board",
		});
	});
});

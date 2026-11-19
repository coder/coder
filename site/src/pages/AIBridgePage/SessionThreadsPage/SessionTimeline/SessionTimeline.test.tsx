import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockAIBridgeThread, MockSession } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { SessionTimeline } from "./SessionTimeline";

describe(SessionTimeline.name, () => {
	it("exposes threads as an ordered list", () => {
		renderComponent(
			<SessionTimeline
				initiator={MockSession.initiator}
				threads={[
					{ ...MockAIBridgeThread, id: "thread-1" },
					{ ...MockAIBridgeThread, id: "thread-2" },
					{ ...MockAIBridgeThread, id: "thread-3" },
				]}
				networkCalls={[]}
				searchQuery=""
				hasNextPage={false}
				isFetchingNextPage={false}
				onFetchNextPage={vi.fn()}
			/>,
		);

		const threads = screen.getByRole("list", { name: "Threads" });
		expect(threads.tagName).toBe("OL");
		expect(within(threads).getAllByRole("listitem")).toHaveLength(3);
	});

	it("gives repeated tool calls distinct names and exposes their expanded state", async () => {
		const user = userEvent.setup();
		const [action] = MockAIBridgeThread.agentic_actions;
		const [toolCall] = action.tool_calls;
		const readFileAction = (id: string) => ({
			...action,
			tool_calls: [{ ...toolCall, id, tool: "read_file" }],
		});
		renderComponent(
			<SessionTimeline
				initiator={MockSession.initiator}
				threads={[
					{
						...MockAIBridgeThread,
						agentic_actions: [
							readFileAction("read-1"),
							readFileAction("read-2"),
						],
					},
				]}
				networkCalls={[]}
				searchQuery=""
				hasNextPage={false}
				isFetchingNextPage={false}
				onFetchNextPage={vi.fn()}
			/>,
		);

		const loopToggle = screen.getByRole("button", { name: /Agentic loop/ });
		expect(loopToggle).toHaveAttribute("aria-expanded", "false");
		await user.click(loopToggle);
		expect(loopToggle).toHaveAttribute("aria-expanded", "true");

		screen.getByRole("button", { name: "Tool call read_file 1 of 2" });
		const secondToggle = screen.getByRole("button", {
			name: "Tool call read_file 2 of 2",
		});
		expect(secondToggle).toHaveAttribute("aria-expanded", "false");
		await user.click(secondToggle);
		expect(secondToggle).toHaveAttribute("aria-expanded", "true");
	});

	it("lets keyboard users focus the tool call input to scroll it", async () => {
		const user = userEvent.setup();
		renderComponent(
			<SessionTimeline
				initiator={MockSession.initiator}
				threads={[MockAIBridgeThread]}
				networkCalls={[]}
				searchQuery=""
				hasNextPage={false}
				isFetchingNextPage={false}
				onFetchNextPage={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Agentic loop/ }));
		await user.click(screen.getByRole("button", { name: /Tool call/ }));
		await user.tab();
		await user.tab();
		expect(
			screen.getByRole("region", { name: "list_directory input" }),
		).toHaveFocus();
	});
});

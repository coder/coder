import { screen, within } from "@testing-library/react";
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
});

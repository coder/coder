import { screen } from "@testing-library/react";
import type { Line } from "#/components/Logs/LogLine";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { AgentLogLine } from "./AgentLogLine";

const line: Line = {
	id: 1,
	level: "info",
	output: 'safe <span data-testid="agent-log-xss">xss</span>',
	sourceId: "source-id",
	time: "2024-03-14T11:31:04.090715Z",
};

describe("AgentLogLine", () => {
	it("renders log HTML as escaped text", () => {
		renderComponent(<AgentLogLine line={line} sourceIcon={null} style={{}} />);

		expect(screen.queryByTestId("agent-log-xss")).not.toBeInTheDocument();
		expect(
			screen.getByText(/safe <span data-testid="agent-log-xss">xss<\/span>/),
		).toBeInTheDocument();
	});
});

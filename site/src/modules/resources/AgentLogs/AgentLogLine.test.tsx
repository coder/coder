import { screen } from "@testing-library/react";
import type { Line } from "#/components/Logs/LogLine";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { AgentLogLine } from "./AgentLogLine";
import { loadGhostty } from "./AnsiText";

beforeAll(async () => {
	// Load the WASM parser up front so lines render styled output
	// synchronously instead of falling back to raw text.
	await loadGhostty();
});

const makeLine = (output: string): Line => ({
	id: 1,
	level: "info",
	output,
	sourceId: "source-id",
	time: "2024-03-14T11:31:04.090715Z",
});

const renderLine = (output: string) =>
	renderComponent(
		<AgentLogLine line={makeLine(output)} sourceIcon={null} style={{}} />,
	);

describe("AgentLogLine", () => {
	it("renders log HTML as escaped text", () => {
		renderLine('safe <span data-testid="agent-log-xss">xss</span>');

		expect(screen.queryByTestId("agent-log-xss")).not.toBeInTheDocument();
		expect(
			screen.getByText(/safe <span data-testid="agent-log-xss">xss<\/span>/),
		).toBeInTheDocument();
	});

	it("renders ANSI color codes as styled markup", () => {
		renderLine("\u001b[31mred\u001b[0m plain");

		const colored = screen.getByText("red");
		expect(colored.tagName).toBe("SPAN");
		expect(colored.getAttribute("style")).toContain("color:");
		expect(screen.getByText(/plain/)).toBeInTheDocument();
	});

	it("emulates carriage return overwrites", () => {
		renderLine("downloading... 50%\rdownloading... 100%");

		expect(screen.getByText("downloading... 100%")).toBeInTheDocument();
		expect(screen.queryByText(/50%/)).not.toBeInTheDocument();
	});

	it("only overwrites the characters a carriage return rewrites", () => {
		// A real terminal moves the cursor to column 0 and overwrites in
		// place; the remainder of the longer first write stays visible.
		renderLine("1234567890\rab");

		expect(screen.getByText("ab34567890")).toBeInTheDocument();
	});

	it("renders a ReDoS payload without hanging", () => {
		// Cure53 CDM-02-004: this pattern caused catastrophic backtracking in
		// ansi-to-html. libghostty-vt is a state-machine parser with no
		// regex, so it parses in linear time.
		const start = performance.now();
		renderLine(`\u001b[${"1".repeat(50000)}`);
		expect(performance.now() - start).toBeLessThan(1000);
	});
});

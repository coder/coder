import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TerminalOutput } from "./TerminalOutput";

describe("TerminalOutput", () => {
	it("pins the command above the output so it stays visible while scrolling", () => {
		render(
			<TerminalOutput ariaLabel="Command output" command="pnpm test">
				output text
			</TerminalOutput>,
		);
		expect(screen.getByText("pnpm test")).toBeInTheDocument();
	});

	it("omits the command bar when no command is known", () => {
		render(
			<TerminalOutput ariaLabel="Process output">output text</TerminalOutput>,
		);
		expect(screen.queryByText("$")).not.toBeInTheDocument();
	});

	it("omits the command bar for whitespace-only commands", () => {
		render(
			<TerminalOutput ariaLabel="Process output" command="  ">
				output text
			</TerminalOutput>,
		);
		expect(screen.queryByText("$")).not.toBeInTheDocument();
	});

	it("focuses the labeled output region from the keyboard", async () => {
		const user = userEvent.setup();
		render(
			<TerminalOutput ariaLabel="Command output" command="pnpm test">
				output text
			</TerminalOutput>,
		);
		const region = screen.getByRole("region", { name: "Command output" });
		await user.tab();
		expect(region).toHaveFocus();
	});

	it("does not show the streaming cursor once the call finishes", () => {
		const { rerender } = render(
			<TerminalOutput ariaLabel="Command output" command="pnpm test" streaming>
				output text
			</TerminalOutput>,
		);
		expect(screen.getByTestId("terminal-streaming-cursor")).toBeInTheDocument();

		rerender(
			<TerminalOutput ariaLabel="Command output" command="pnpm test">
				output text
			</TerminalOutput>,
		);
		expect(
			screen.queryByTestId("terminal-streaming-cursor"),
		).not.toBeInTheDocument();
	});
});

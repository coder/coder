import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ProcessOutputTool } from "./ProcessOutputTool";

describe("ProcessOutputTool rendering", () => {
	it("labels a completed poll of a live process as checked-on, still running", () => {
		renderComponent(
			<ProcessOutputTool
				output="> Starting Vite dev server..."
				command="npm start"
				status="completed"
				processRunning
				exitCode={null}
				isError={false}
				processId="376b2458"
			/>,
		);

		const header = screen.getByRole("button", {
			name: /Expand process output/,
		});
		expect(header).toHaveTextContent("Checked on npm start");
		expect(header).toHaveTextContent("still running");
	});

	it("labels a poll in flight as checking-on", () => {
		renderComponent(
			<ProcessOutputTool
				output="> Starting Vite dev server..."
				command="npm start"
				status="running"
				exitCode={null}
				isError={false}
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		expect(
			screen.getByRole("button", { name: /Expand process output/ }),
		).toHaveTextContent("Checking on npm start");
	});

	it("labels a clean exit as finished", () => {
		renderComponent(
			<ProcessOutputTool
				output="build completed"
				command="go build ./..."
				status="completed"
				processRunning={false}
				exitCode={0}
				isError={false}
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		const header = screen.getByRole("button", {
			name: /Expand process output/,
		});
		expect(header).toHaveTextContent("Checked on go build ./...");
		expect(header).toHaveTextContent("finished");
		// A clean exit is the expected outcome, so no exit badge.
		expect(screen.queryByText("exit 0")).not.toBeInTheDocument();
	});

	it("surfaces no-new-output in the header and keeps the buffer on expand", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ProcessOutputTool
				output="=== RUN TestFoo"
				command="go test ./..."
				status="completed"
				processRunning
				exitCode={null}
				isError={false}
				noNewOutput
			/>,
		);

		const header = screen.getByRole("button", {
			name: /Expand process output/,
		});
		expect(header).toHaveTextContent("no new output");

		await user.click(header);
		// Expanding reveals the quiet note and the unchanged buffer.
		screen.getByText("No new output since the last check");
		screen.getByText("=== RUN TestFoo");
	});

	it("omits the shell prompt header from the output pane", () => {
		renderComponent(
			<ProcessOutputTool
				output="build completed"
				command="go build ./..."
				status="completed"
				processRunning={false}
				exitCode={0}
				isError={false}
				shellToolDisplayMode="always_expanded"
			/>,
		);

		expect(screen.queryByText("$")).not.toBeInTheDocument();
	});

	it("renders a truncation notice when the buffer was cut", () => {
		renderComponent(
			<ProcessOutputTool
				output="first\n...snip...\nlast"
				command="npm run dev"
				status="completed"
				processRunning={false}
				exitCode={0}
				isError={false}
				shellToolDisplayMode="always_expanded"
				truncation={{
					original_bytes: 1_200_000,
					retained_bytes: 32_768,
					omitted_bytes: 1_167_232,
					strategy: "head-tail",
				}}
			/>,
		);

		expect(
			screen.getByText("Output truncated; showing 32 KB of 1.1 MB"),
		).toHaveTextContent("Output truncated");
	});

	it("keeps the model intent as the primary label with the status suffix", () => {
		renderComponent(
			<ProcessOutputTool
				output="> Starting Vite dev server..."
				command="npm start"
				modelIntent="Waiting for the dev server to be ready"
				status="completed"
				processRunning
				exitCode={null}
				isError={false}
			/>,
		);

		const header = screen.getByRole("button", {
			name: /Expand process output/,
		});
		expect(header).toHaveTextContent("Waiting for the dev server to be ready");
		expect(header).toHaveTextContent("still running");
	});
});

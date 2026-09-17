import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ExecuteTool } from "./ExecuteTool";

const timeoutBlocks = [{ kind: "output" as const, text: "=== RUN TestFoo" }];

describe("ExecuteTool timeout rendering", () => {
	it("labels a timed-out run as started with a plain-text still-running suffix", () => {
		renderComponent(
			<ExecuteTool
				command="go test ./..."
				transcriptBlocks={timeoutBlocks}
				status="completed"
				isError={false}
				processId="376b2458"
				timedOut
				processRunning
				waitLimit="30s"
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		expect(
			screen.getByRole("button", { name: /Expand command/ }),
		).toHaveTextContent("Started go test ./...");
		expect(
			screen.getByRole("img", { name: /Stopped waiting after 30s/ }),
		).toHaveTextContent("· still running");
		expect(
			screen.getByRole("img", { name: /Process 376b2458/ }),
		).toHaveTextContent("376b2458");
		// The timeout string is control metadata and never reaches the user.
		expect(screen.queryByText(/command timed out/)).not.toBeInTheDocument();
	});

	it("omits the duration suffix for timed-out runs", () => {
		renderComponent(
			<ExecuteTool
				command="go test ./..."
				transcriptBlocks={timeoutBlocks}
				status="completed"
				isError={false}
				durationMs={47200}
				timedOut
				processRunning
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		expect(
			screen.getByRole("button", { name: /Expand command/ }),
		).not.toHaveTextContent(/for 47\.2s/);
	});

	it("shows the status-unknown suffix when liveness is unconfirmed", () => {
		renderComponent(
			<ExecuteTool
				command="go test ./..."
				transcriptBlocks={[]}
				status="completed"
				isError={false}
				processId="376b2458"
				timedOut
				processRunning={false}
				waitLimit="30s"
			/>,
		);

		expect(
			screen.getByRole("img", { name: /could not read the process state/ }),
		).toHaveTextContent("· status unknown");
	});

	it("renders the wait-limit note in the expanded transcript", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ExecuteTool
				command="go test ./..."
				transcriptBlocks={timeoutBlocks}
				status="completed"
				isError={false}
				timedOut
				processRunning
				waitLimit="30s"
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		await user.click(screen.getByRole("button", { name: /Expand command/ }));
		expect(
			screen.getByText(
				"Reached the 30s wait limit; the process was not stopped.",
			),
		).toBeInTheDocument();
	});

	it("keeps the failure verb and error block for genuine failures", () => {
		renderComponent(
			<ExecuteTool
				command="make build"
				transcriptBlocks={[{ kind: "error" as const, text: "exit status 2" }]}
				status="error"
				isError
				errorText="exit status 2"
				shellToolDisplayMode="always_collapsed"
			/>,
		);

		expect(
			screen.getByRole("button", { name: /Expand command/ }),
		).toHaveTextContent("Failed to run make build");
		// The warning status is named by the error message.
		screen.getByRole("img", { name: "exit status 2" });
	});
});

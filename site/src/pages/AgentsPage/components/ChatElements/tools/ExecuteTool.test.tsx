import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ExecuteTool } from "./ExecuteTool";

const renderTool = (props: {
	status: "running" | "completed";
	startedAt?: string;
}) =>
	render(
		<TooltipProvider>
			<ExecuteTool
				command="make build"
				transcriptBlocks={[]}
				isError={false}
				{...props}
			/>
		</TooltipProvider>,
	);

describe("ExecuteTool elapsed time", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it("counts from the tool call's created_at and ticks while running", () => {
		vi.useFakeTimers({ now: new Date("2025-01-01T00:01:05.000Z") });
		renderTool({ status: "running", startedAt: "2025-01-01T00:00:00.000Z" });

		expect(screen.getByTitle("Elapsed time")).toHaveTextContent("1m 5s");

		act(() => {
			vi.advanceTimersByTime(55_000);
		});
		expect(screen.getByTitle("Elapsed time")).toHaveTextContent("2m");
	});

	it("falls back to mount time when created_at is missing", () => {
		vi.useFakeTimers();
		renderTool({ status: "running" });

		expect(screen.getByTitle("Elapsed time")).toHaveTextContent("0s");

		act(() => {
			vi.advanceTimersByTime(7_000);
		});
		expect(screen.getByTitle("Elapsed time")).toHaveTextContent("7s");
	});

	it("hides the readout once the command completes", () => {
		renderTool({ status: "completed", startedAt: "2025-01-01T00:00:00.000Z" });
		expect(screen.queryByTitle("Elapsed time")).not.toBeInTheDocument();
	});
});

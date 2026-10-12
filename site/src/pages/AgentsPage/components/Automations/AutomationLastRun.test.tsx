import { render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { describe, expect, it } from "vitest";
import { ChatAutomationRunReasons } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	AutomationLastRun,
	type AutomationLastRunValue,
	describeLastRunOutcome,
	describeRunReason,
	runReasonText,
} from "./AutomationLastRun";

const now = new Date("2026-10-12T12:00:00Z");
const fiveMinutesAgo = "2026-10-12T11:55:00Z";

// Reasons that resolve without a change; every other reason is a refusal.
const skippedReasons = [
	"chat_busy",
	"queue_full",
	"missed",
	"previous_run_active",
];

const renderLastRun = (lastRun: AutomationLastRunValue | null) => {
	const router = createMemoryRouter([
		{
			path: "/",
			element: (
				<TooltipProvider>
					<AutomationLastRun lastRun={lastRun} now={now} showHint />
				</TooltipProvider>
			),
		},
	]);
	return render(<RouterProvider router={router} />);
};

describe("runReasonText", () => {
	it("words every reason the server can send", () => {
		// previous_run_active is listed ahead of the generated type.
		expect(Object.keys(runReasonText).sort()).toEqual(
			[...new Set([...ChatAutomationRunReasons, "previous_run_active"])].sort(),
		);
	});

	it("tells the owner what to do for every refusal", () => {
		const missingHints = ChatAutomationRunReasons.filter(
			(reason) =>
				!skippedReasons.includes(reason) && !runReasonText[reason].hint,
		);
		const skippedWithHints = skippedReasons.filter(
			(reason) => describeRunReason(reason).hint,
		);
		expect(skippedWithHints).toEqual([]);
		expect(missingHints).toEqual([]);
	});

	it("falls back to the raw value for a reason this UI does not know", () => {
		expect(describeRunReason("rate_limited")).toEqual({
			label: "rate limited",
		});
	});
});

describe("describeLastRunOutcome", () => {
	it.each([
		[{ outcome: "accepted" }, "Accepted"],
		[{ outcome: "skipped", reason: "chat_busy" }, "Skipped: chat busy"],
		[{ outcome: "refused", reason: "hook_denied" }, "Refused: hook denied"],
	] as const)("describes %o as %s", (run, text) => {
		expect(describeLastRunOutcome({ at: fiveMinutesAgo, ...run })).toBe(text);
	});
});

describe("AutomationLastRun", () => {
	it.each([
		[null, "No runs yet"],
		[
			{ at: fiveMinutesAgo, outcome: "accepted" },
			"Last run 5 minutes ago · Accepted",
		],
		[
			{ at: fiveMinutesAgo, outcome: "skipped", reason: "queue_full" },
			"Last run 5 minutes ago · Skipped: queue full",
		],
		[
			{
				at: fiveMinutesAgo,
				outcome: "skipped",
				reason: "previous_run_active",
				chat_id: "chat-active",
			},
			"Last run 5 minutes ago · Skipped: previous run still running · Open chat",
		],
		[
			{ at: fiveMinutesAgo, outcome: "refused", reason: "model_unavailable" },
			"Last run 5 minutes ago · Refused: model unavailableThe model is turned off or was removed. Pick another model.",
		],
	] as const)("reads %o as %s", (lastRun, text) => {
		const { container } = renderLastRun(lastRun);
		expect(container.textContent).toBe(text);
	});

	it("links to the chat that received the run", () => {
		renderLastRun({
			at: fiveMinutesAgo,
			outcome: "accepted",
			chat_id: "chat-1",
		});
		expect(
			screen
				.getByRole("link", { name: "Open chat of the last run" })
				.getAttribute("href"),
		).toBe("/agents/chat-1");
	});
});

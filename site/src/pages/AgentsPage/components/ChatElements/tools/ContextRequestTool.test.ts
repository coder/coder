import { describe, expect, it } from "vitest";
import {
	getContextRequestErrorMessage,
	getContextRequestFollowUp,
	getContextRequestLabel,
	isContextRequestExpandedByDefault,
} from "./ContextRequestTool";

describe("getContextRequestFollowUp", () => {
	it("prefers the follow_up call argument", () => {
		expect(
			getContextRequestFollowUp(
				JSON.stringify({ follow_up: "  Resume from PLAN.md step 3.  " }),
				{ output: "Context cleared. Follow-up: something else" },
			),
		).toBe("Resume from PLAN.md step 3.");
	});

	it("accepts object args", () => {
		expect(
			getContextRequestFollowUp({ follow_up: "Continue the review." }, null),
		).toBe("Continue the review.");
	});

	it("falls back to the echo in the result output", () => {
		expect(
			getContextRequestFollowUp(undefined, {
				output: "Compaction scheduled. Follow-up: Push branch scott/demo.",
			}),
		).toBe("Push branch scott/demo.");
	});

	it("keeps later occurrences of the echo marker inside the note", () => {
		expect(
			getContextRequestFollowUp(undefined, {
				output: "Context cleared. Follow-up: First. Follow-up: second.",
			}),
		).toBe("First. Follow-up: second.");
	});

	it("returns an empty string when neither source carries a note", () => {
		expect(getContextRequestFollowUp(undefined, undefined)).toBe("");
		expect(getContextRequestFollowUp("{}", { error: "rejected" })).toBe("");
		expect(
			getContextRequestFollowUp(undefined, { output: "no marker here" }),
		).toBe("");
	});
});

describe("getContextRequestErrorMessage", () => {
	it("reads the error field of a record result", () => {
		expect(
			getContextRequestErrorMessage({ error: "follow_up is required" }, true),
		).toBe("follow_up is required");
	});

	it("falls back to the message field", () => {
		expect(
			getContextRequestErrorMessage({ error: "", message: "denied" }, true),
		).toBe("denied");
	});

	it("uses a string result only when the call is flagged as an error", () => {
		expect(getContextRequestErrorMessage("boom", true)).toBe("boom");
		expect(getContextRequestErrorMessage("boom", false)).toBeUndefined();
	});

	it("returns undefined for success results and empty values", () => {
		expect(
			getContextRequestErrorMessage({ output: "Context cleared." }, false),
		).toBeUndefined();
		expect(getContextRequestErrorMessage(undefined, true)).toBeUndefined();
		expect(getContextRequestErrorMessage("", true)).toBeUndefined();
	});
});

describe("getContextRequestLabel", () => {
	it.each([
		["clear", "running", false, "Clearing context…"],
		["compact", "running", false, "Compacting context…"],
		["clear", "completed", false, "Context clear requested"],
		["compact", "completed", false, "Compaction requested"],
		["clear", "completed", true, "Context clear rejected"],
		["compact", "completed", true, "Compaction rejected"],
		["clear", "error", false, "Context clear rejected"],
		["compact", "error", false, "Compaction rejected"],
	] as const)(
		"labels kind=%s status=%s isError=%s as %j",
		(kind, status, isError, label) => {
			expect(getContextRequestLabel({ kind, status, isError })).toBe(label);
		},
	);
});

describe("isContextRequestExpandedByDefault", () => {
	it.each([
		["running", false, true],
		["completed", false, false],
		["completed", true, true],
		["error", false, true],
		["error", true, true],
	] as const)(
		"status=%s isError=%s expands by default: %s",
		(status, isError, expanded) => {
			expect(isContextRequestExpandedByDefault({ status, isError })).toBe(
				expanded,
			);
		},
	);
});

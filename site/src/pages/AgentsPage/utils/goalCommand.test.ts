import { describe, expect, it } from "vitest";
import { parseGoalCommand } from "./goalCommand";

describe("parseGoalCommand", () => {
	it("ignores non-goal messages and inline slash triggers", () => {
		expect(parseGoalCommand("please /goal fix the build")).toBeNull();
		expect(parseGoalCommand("/goalie fix the build")).toBeNull();
	});

	it("parses /goal as a show command", () => {
		expect(parseGoalCommand("/goal")).toEqual({ kind: "show" });
		expect(parseGoalCommand("/goal   ")).toEqual({ kind: "show" });
	});

	it("parses an objective as a set mutation", () => {
		expect(parseGoalCommand("/goal fix the flaky tests")).toEqual({
			kind: "set",
			objective: "fix the flaky tests",
			mutation: { action: "set", objective: "fix the flaky tests" },
		});
	});

	it("parses escaped objectives with double dash", () => {
		expect(parseGoalCommand("/goal -- clear the cache")).toEqual({
			kind: "set",
			objective: "clear the cache",
			mutation: { action: "set", objective: "clear the cache" },
		});
	});

	it("parses lifecycle commands", () => {
		expect(parseGoalCommand("/goal clear")).toEqual({
			kind: "lifecycle",
			action: "clear",
			mutation: { action: "clear" },
		});
		expect(parseGoalCommand("/goal pause")).toEqual({
			kind: "lifecycle",
			action: "pause",
			mutation: { action: "pause" },
		});
		expect(parseGoalCommand("/goal resume")).toEqual({
			kind: "lifecycle",
			action: "resume",
			mutation: { action: "resume" },
		});
	});

	it("parses complete with and without a summary", () => {
		expect(parseGoalCommand("/goal complete")).toEqual({
			kind: "lifecycle",
			action: "complete",
			mutation: { action: "complete" },
		});
		expect(parseGoalCommand("/goal complete --summary Fixed the bug")).toEqual({
			kind: "lifecycle",
			action: "complete",
			mutation: { action: "complete", completion_summary: "Fixed the bug" },
		});
	});

	it("rejects unsupported budget commands", () => {
		expect(parseGoalCommand("/goal budget 10 turns")).toMatchObject({
			kind: "unsupported",
		});
		expect(parseGoalCommand("/goal --budget 10")).toMatchObject({
			kind: "unsupported",
		});
	});

	it("rejects unsupported turn cap flags", () => {
		expect(parseGoalCommand("/goal --turns 5 fix it")).toMatchObject({
			kind: "unsupported",
		});
		expect(parseGoalCommand("/goal --max-turns 5 fix it")).toMatchObject({
			kind: "unsupported",
		});
		expect(parseGoalCommand("/goal --turn-limit 5 fix it")).toMatchObject({
			kind: "unsupported",
		});
	});

	it("preserves multiline objectives", () => {
		expect(parseGoalCommand("/goal fix the tests\nthen run lint")).toEqual({
			kind: "set",
			objective: "fix the tests\nthen run lint",
			mutation: { action: "set", objective: "fix the tests\nthen run lint" },
		});
	});

	it("reserves a start-of-message /goal command over a personal skill name", () => {
		expect(parseGoalCommand("/goal review this")).toMatchObject({
			kind: "set",
			objective: "review this",
		});
		expect(parseGoalCommand("Use the /goal skill inline")).toBeNull();
	});
});

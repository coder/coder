import { describe, expect, it } from "vitest";
import {
	MockChatProjectSlackTrigger,
	MockChatProjectTrigger,
	MockChatProjectTriggerDisabled,
} from "#/testHelpers/entities";
import {
	hasUnverifiedFilters,
	suggestTriggerName,
	triggerNameMaxLength,
	validateTriggerTextPattern,
} from "./triggerCatalog";

describe("hasUnverifiedFilters", () => {
	it("flags sender, channel, and repository filters without a secret", () => {
		const unfilteredGitHub = { ...MockChatProjectTrigger, repository: "" };
		expect(hasUnverifiedFilters(MockChatProjectTrigger)).toBe(true);
		expect(hasUnverifiedFilters(unfilteredGitHub)).toBe(false);
		expect(
			hasUnverifiedFilters({ ...unfilteredGitHub, senders: ["octocat"] }),
		).toBe(true);
		expect(
			hasUnverifiedFilters({
				...MockChatProjectSlackTrigger,
				has_slack_signing_secret: false,
			}),
		).toBe(true);
	});

	it("trusts filters once the source's secret is set", () => {
		expect(
			hasUnverifiedFilters({
				...MockChatProjectTrigger,
				has_github_webhook_secret: true,
			}),
		).toBe(false);
		expect(hasUnverifiedFilters(MockChatProjectSlackTrigger)).toBe(false);
		// A Slack secret does not verify a GitHub trigger.
		expect(
			hasUnverifiedFilters({
				...MockChatProjectTrigger,
				has_slack_signing_secret: true,
			}),
		).toBe(true);
	});

	it("never flags plain webhooks, which have no secret", () => {
		expect(
			hasUnverifiedFilters({
				...MockChatProjectTriggerDisabled,
				senders: ["someone"],
			}),
		).toBe(false);
	});
});

describe("validateTriggerTextPattern", () => {
	it("accepts RE2 flag and named groups that JavaScript spells differently", () => {
		expect(
			validateTriggerTextPattern("(?i)^/deploy (?P<env>\\w+)", true),
		).toBeUndefined();
		expect(validateTriggerTextPattern("(?s:a.b)", true)).toBeUndefined();
	});

	it("rejects a broken regex but not the same text without regex", () => {
		expect(validateTriggerTextPattern("C++", true)).toBeDefined();
		expect(validateTriggerTextPattern("C++", false)).toBeUndefined();
	});
});

describe("suggestTriggerName", () => {
	it("cuts long names by characters, not UTF-16 code units", () => {
		const name = suggestTriggerName({
			source: "slack",
			events: ["message"],
			repository: "",
			channel: "😀".repeat(triggerNameMaxLength),
		});

		expect(Array.from(name)).toHaveLength(triggerNameMaxLength);
		expect(name.endsWith("😀")).toBe(true);
	});
});

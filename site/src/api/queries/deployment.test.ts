import { describe, expect, it } from "vitest";
import type { UsagePeriod } from "#/api/typesGenerated";
import { deploymentAgentTime } from "./deployment";

const usagePeriod: UsagePeriod = {
	issued_at: "2026-08-01T00:00:00Z",
	start: "2026-08-01T00:00:00Z",
	end: "2026-09-01T00:00:00Z",
};

describe("deploymentAgentTime", () => {
	it("keys usage by the license period", () => {
		expect(deploymentAgentTime().queryKey).toEqual(["deployment", "agentTime"]);
		expect(deploymentAgentTime(usagePeriod).queryKey).toEqual([
			"deployment",
			"agentTime",
			usagePeriod.start,
			usagePeriod.end,
		]);
		expect(
			deploymentAgentTime({
				...usagePeriod,
				start: "2026-09-01T00:00:00Z",
				end: "2026-10-01T00:00:00Z",
			}).queryKey,
		).not.toEqual(deploymentAgentTime(usagePeriod).queryKey);
	});
});

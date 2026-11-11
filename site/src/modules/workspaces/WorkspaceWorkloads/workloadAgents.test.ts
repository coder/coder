import { describe, expect, it } from "vitest";
import {
	MockWorkloadAgent,
	MockWorkloadShapeAgent,
	MockWorkspaceAgent,
	MockWorkspaceResource,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import { hasWorkloadShape, withoutWorkloadAgents } from "./workloadAgents";

describe("workload agents", () => {
	it("leaves a workspace without a shape unchanged", () => {
		const resources = [
			{
				...MockWorkspaceResource,
				agents: [MockWorkspaceAgent, MockWorkspaceSubAgent],
			},
			{ ...MockWorkspaceResource, id: "no-agents", agents: undefined },
		];

		expect(hasWorkloadShape(resources)).toBe(false);
		expect(withoutWorkloadAgents(resources)).toEqual(resources);
	});

	it("removes shapes and their daemons, and keeps every other agent", () => {
		const resources = [
			{
				...MockWorkspaceResource,
				id: "human",
				agents: [MockWorkspaceAgent, MockWorkspaceSubAgent],
			},
			{
				...MockWorkspaceResource,
				id: "shape",
				agents: [MockWorkloadShapeAgent, MockWorkloadAgent],
			},
		];

		expect(hasWorkloadShape(resources)).toBe(true);
		expect(
			withoutWorkloadAgents(resources).map((resource) => ({
				id: resource.id,
				agents: resource.agents?.map((agent) => agent.id),
			})),
		).toEqual([
			{
				id: "human",
				agents: [MockWorkspaceAgent.id, MockWorkspaceSubAgent.id],
			},
			{ id: "shape", agents: [] },
		]);
	});
});

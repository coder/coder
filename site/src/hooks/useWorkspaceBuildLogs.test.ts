import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { MockWorkspaceBuildLogs } from "#/testHelpers/entities";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { useWorkspaceBuildLogs } from "./useWorkspaceBuildLogs";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useWorkspaceBuildLogs", () => {
	it("keeps logs while disabled and starts each new build from empty logs", () => {
		const publishLog = new Map<string, (log: ProvisionerJobLog) => void>();
		vi.spyOn(apiModule, "watchBuildLogsByBuildId").mockImplementation(
			(buildId, { onMessage }) => {
				publishLog.set(buildId, onMessage);
				return createMockWebSocket("ws://test")[0];
			},
		);
		const [firstLog, secondLog] = MockWorkspaceBuildLogs;

		const { result, rerender } = renderHook(
			({ buildId, enabled }) => useWorkspaceBuildLogs(buildId, enabled),
			{ initialProps: { buildId: "build-1", enabled: true } },
		);
		act(() => publishLog.get("build-1")?.(firstLog));
		expect(result.current).toEqual([firstLog]);

		rerender({ buildId: "build-1", enabled: false });
		expect(result.current).toEqual([firstLog]);

		rerender({ buildId: "build-2", enabled: true });
		expect(result.current).toBeUndefined();

		act(() => publishLog.get("build-2")?.(secondLog));
		act(() => publishLog.get("build-1")?.(firstLog));
		expect(result.current).toEqual([secondLog]);
	});
});

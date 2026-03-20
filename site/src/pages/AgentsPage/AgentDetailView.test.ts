import { describe, expect, it, vi } from "vitest";
import {
	type AgentDetailSidebarModules,
	createAgentDetailSidebarModulesLoader,
} from "./AgentDetailView";

const makeModules = () => [{}, {}, {}] as unknown as AgentDetailSidebarModules;

describe("createAgentDetailSidebarModulesLoader", () => {
	it("reuses the same in-flight import promise", async () => {
		const modules = makeModules();
		let resolveLoad: (value: AgentDetailSidebarModules) => void = () => {};
		const loadModules = vi.fn(
			() =>
				new Promise<AgentDetailSidebarModules>((resolve) => {
					resolveLoad = resolve;
				}),
		);
		const loadSidebarModules =
			createAgentDetailSidebarModulesLoader(loadModules);

		const firstPromise = loadSidebarModules();
		const secondPromise = loadSidebarModules();

		expect(firstPromise).toBe(secondPromise);
		expect(loadModules).toHaveBeenCalledTimes(1);

		resolveLoad(modules);

		await expect(firstPromise).resolves.toBe(modules);
		await expect(secondPromise).resolves.toBe(modules);
	});

	it("clears rejected imports so later calls can retry", async () => {
		const modules = makeModules();
		const loadModules = vi
			.fn<() => Promise<AgentDetailSidebarModules>>()
			.mockRejectedValueOnce(new Error("chunk load failed"))
			.mockResolvedValueOnce(modules);
		const loadSidebarModules =
			createAgentDetailSidebarModulesLoader(loadModules);

		await expect(loadSidebarModules()).rejects.toThrow("chunk load failed");
		await expect(loadSidebarModules()).resolves.toBe(modules);
		await expect(loadSidebarModules()).resolves.toBe(modules);
		expect(loadModules).toHaveBeenCalledTimes(2);
	});
});

import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
	type AgentDetailSidebarComponents,
	type AgentDetailSidebarModules,
	createAgentDetailSidebarModulesLoader,
	DeferredAgentDetailSidebar,
} from "./AgentDetailView";

const makeModules = () => [{}, {}, {}] as unknown as AgentDetailSidebarModules;

const makeSidebarComponents = () =>
	({
		RightPanel: (() =>
			null) as unknown as AgentDetailSidebarComponents["RightPanel"],
		SidebarTabView: (() =>
			null) as unknown as AgentDetailSidebarComponents["SidebarTabView"],
		GitPanel: (() =>
			null) as unknown as AgentDetailSidebarComponents["GitPanel"],
	}) satisfies AgentDetailSidebarComponents;

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

describe("DeferredAgentDetailSidebar", () => {
	it("retries a failed sidebar load after the panel is reopened", async () => {
		const sidebarComponents = makeSidebarComponents();
		const loadSidebarComponents = vi
			.fn<() => Promise<AgentDetailSidebarComponents>>()
			.mockRejectedValueOnce(new Error("chunk load failed"))
			.mockResolvedValueOnce(sidebarComponents);

		const renderSidebar = (isOpen: boolean) => (
			<DeferredAgentDetailSidebar
				isOpen={isOpen}
				loadSidebarComponents={loadSidebarComponents}
				fallback={<div>loading sidebar</div>}
			>
				{() => <div>loaded sidebar</div>}
			</DeferredAgentDetailSidebar>
		);

		const { rerender } = render(renderSidebar(true));

		expect(screen.getByText("loading sidebar")).toBeInTheDocument();
		await waitFor(() => expect(loadSidebarComponents).toHaveBeenCalledTimes(1));
		await waitFor(() => {
			expect(screen.queryByText("loaded sidebar")).not.toBeInTheDocument();
		});

		rerender(renderSidebar(false));
		expect(screen.queryByText("loading sidebar")).not.toBeInTheDocument();

		rerender(renderSidebar(true));
		await waitFor(() => expect(loadSidebarComponents).toHaveBeenCalledTimes(2));
		await screen.findByText("loaded sidebar");
	});
});

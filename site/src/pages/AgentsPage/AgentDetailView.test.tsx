import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	type AgentDetailSidebarComponents,
	type AgentDetailSidebarModules,
	AgentDetailSidebarPanelLoadingFallback,
	createAgentDetailSidebarModulesLoader,
	DeferredAgentDetailSidebar,
} from "./AgentDetailView";
import {
	RIGHT_PANEL_DEFAULT_WIDTH,
	RIGHT_PANEL_WIDTH_STORAGE_KEY,
} from "./rightPanelState";

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

beforeEach(() => {
	localStorage.clear();
});

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

describe("AgentDetailSidebarPanelLoadingFallback", () => {
	it("uses the persisted right panel width", () => {
		localStorage.setItem(RIGHT_PANEL_WIDTH_STORAGE_KEY, "640");

		const { container } = render(<AgentDetailSidebarPanelLoadingFallback />);
		const fallback = container.firstElementChild;

		expect(fallback).toHaveStyle({ "--panel-width": "640px" });
	});

	it("falls back to the default width for invalid persisted values", () => {
		localStorage.setItem(RIGHT_PANEL_WIDTH_STORAGE_KEY, "120");

		const { container } = render(<AgentDetailSidebarPanelLoadingFallback />);
		const fallback = container.firstElementChild;

		expect(fallback).toHaveStyle({
			"--panel-width": `${RIGHT_PANEL_DEFAULT_WIDTH}px`,
		});
	});
});

describe("DeferredAgentDetailSidebar", () => {
	it("offers a close path after a failed sidebar load and retries on reopen", async () => {
		const sidebarComponents = makeSidebarComponents();
		const loadSidebarComponents = vi
			.fn<() => Promise<AgentDetailSidebarComponents>>()
			.mockRejectedValueOnce(new Error("chunk load failed"))
			.mockResolvedValueOnce(sidebarComponents);

		const SidebarHarness = () => {
			const [isOpen, setIsOpen] = useState(true);

			return (
				<>
					<button type="button" onClick={() => setIsOpen(true)}>
						Reopen sidebar
					</button>
					<DeferredAgentDetailSidebar
						isOpen={isOpen}
						loadSidebarComponents={loadSidebarComponents}
						fallback={<div>loading sidebar</div>}
						loadFailureFallback={({ retry }) => (
							<div>
								<button type="button" onClick={retry}>
									Retry sidebar
								</button>
								<button type="button" onClick={() => setIsOpen(false)}>
									Close panel
								</button>
							</div>
						)}
					>
						{() => <div>loaded sidebar</div>}
					</DeferredAgentDetailSidebar>
				</>
			);
		};

		render(<SidebarHarness />);

		expect(screen.getByText("loading sidebar")).toBeInTheDocument();
		await waitFor(() => expect(loadSidebarComponents).toHaveBeenCalledTimes(1));
		await screen.findByRole("button", { name: "Close panel" });
		expect(screen.queryByText("loaded sidebar")).not.toBeInTheDocument();

		fireEvent.click(screen.getByRole("button", { name: "Close panel" }));
		await waitFor(() => {
			expect(
				screen.queryByRole("button", { name: "Close panel" }),
			).not.toBeInTheDocument();
		});

		fireEvent.click(screen.getByRole("button", { name: "Reopen sidebar" }));
		await waitFor(() => expect(loadSidebarComponents).toHaveBeenCalledTimes(2));
		await screen.findByText("loaded sidebar");
	});
});

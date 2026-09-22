import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { type SidebarTab, SidebarTabView } from "./SidebarTabView";

const initialTabs: SidebarTab[] = [
	{ id: "git", label: "Git", content: <input aria-label="Git note" /> },
	{
		id: "terminal",
		label: "Terminal",
		content: <input aria-label="Terminal command" />,
	},
	{
		id: "preview",
		label: "Preview",
		content: <input aria-label="Preview URL" />,
	},
];

function renderTabs({ activeTabId = "git", tabs = initialTabs } = {}) {
	const onActiveTabChange = vi.fn();
	const onCloseTab = vi.fn();
	const onToggleExpanded = vi.fn();
	const onAddTab = vi.fn();
	function View() {
		const [visibleTabs, setVisibleTabs] = useState(tabs);
		const [active, setActive] = useState(activeTabId);
		return (
			<>
				<SidebarTabView
					tabs={visibleTabs.map((tab) => ({
						...tab,
						onClose: () => {
							onCloseTab(tab.id);
							const remaining = visibleTabs.filter(({ id }) => id !== tab.id);
							setVisibleTabs(remaining);
							if (active === tab.id) {
								setActive(
									remaining[
										Math.min(visibleTabs.indexOf(tab), remaining.length - 1)
									]?.id ?? "",
								);
							}
						},
					}))}
					effectiveTabId={active || null}
					onActiveTabChange={(id) => {
						onActiveTabChange(id);
						setActive(id);
					}}
					isExpanded={false}
					onToggleExpanded={onToggleExpanded}
					addTabControl={
						<button type="button" onClick={onAddTab}>
							New terminal tab
						</button>
					}
				/>
				<button type="button">After panel</button>
			</>
		);
	}
	render(<View />);
	return { onActiveTabChange, onCloseTab, onToggleExpanded, onAddTab };
}

describe("SidebarTabView", () => {
	it("selects and focuses tabs with arrow, Home, and End keys", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderTabs({ activeTabId: "terminal" });
		await user.tab();
		expect(
			screen.getByRole("tab", { name: "Terminal", selected: true }),
		).toHaveFocus();
		for (const [key, name, id] of [
			["{ArrowRight}", "Preview", "preview"],
			["{ArrowRight}", "Git", "git"],
			["{ArrowLeft}", "Preview", "preview"],
			["{Home}", "Git", "git"],
			["{End}", "Preview", "preview"],
		]) {
			await user.keyboard(key);
			await waitFor(() => {
				expect(onActiveTabChange).toHaveBeenLastCalledWith(id);
				expect(screen.getByRole("tab", { name, selected: true })).toHaveFocus();
			});
		}
	});

	it("keeps inactive tabs out of sequential navigation and leaves toolbar controls independent", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange, onAddTab, onToggleExpanded } = renderTabs({
			activeTabId: "terminal",
		});
		await user.tab();
		expect(screen.getByRole("tab", { name: "Terminal" })).toHaveFocus();
		await user.tab();
		expect(
			screen.getByRole("button", { name: "Close Terminal tab" }),
		).toHaveFocus();
		await user.tab();
		expect(
			screen.getByRole("button", { name: "New terminal tab" }),
		).toHaveFocus();
		await user.keyboard("{ArrowLeft}{Enter}");
		expect(onAddTab).toHaveBeenCalledOnce();
		expect(onActiveTabChange).not.toHaveBeenCalled();
		await user.tab();
		expect(screen.getByRole("button", { name: "Expand panel" })).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(onToggleExpanded).toHaveBeenCalledOnce();
	});

	it("uses the selected tab as the entry point after an external selection change", async () => {
		const user = userEvent.setup();
		function View() {
			const [active, setActive] = useState("git");
			return (
				<>
					<SidebarTabView
						tabs={initialTabs}
						effectiveTabId={active}
						onActiveTabChange={setActive}
						isExpanded={false}
						onToggleExpanded={() => {}}
					/>
					<button type="button" onClick={() => setActive("preview")}>
						Open preview
					</button>
				</>
			);
		}
		render(<View />);
		await user.tab();
		expect(screen.getByRole("tab", { name: "Git" })).toHaveFocus();
		await user.click(screen.getByRole("button", { name: "Open preview" }));
		await user.tab();
		await user.tab();
		expect(
			screen.getByRole("tab", { name: "Preview", selected: true }),
		).toHaveFocus();
	});

	it("preserves panel state when changing tabs", async () => {
		const user = userEvent.setup();
		renderTabs();
		const note = screen.getByRole("textbox", { name: "Git note" });
		await user.type(note, "Uncommitted notes");
		await user.click(screen.getByRole("tab", { name: "Terminal" }));
		await user.type(
			screen.getByRole("textbox", { name: "Terminal command" }),
			"pwd",
		);
		await user.click(screen.getByRole("tab", { name: "Git" }));
		expect(screen.getByRole("textbox", { name: "Git note" })).toBe(note);
		expect(note).toHaveValue("Uncommitted notes");
		await user.click(screen.getByRole("tab", { name: "Terminal" }));
		expect(
			screen.getByRole("textbox", { name: "Terminal command" }),
		).toHaveValue("pwd");
	});

	it("focuses the parent's surviving selection after deleting focused tabs", async () => {
		const user = userEvent.setup();
		const { onCloseTab } = renderTabs({ activeTabId: "terminal" });
		await user.tab();
		await user.keyboard("{Delete}");
		expect(onCloseTab).toHaveBeenLastCalledWith("terminal");
		expect(
			screen.getByRole("tab", { name: "Preview", selected: true }),
		).toHaveFocus();
		await user.keyboard("{Delete}");
		expect(onCloseTab).toHaveBeenLastCalledWith("preview");
		expect(
			screen.getByRole("tab", { name: "Git", selected: true }),
		).toHaveFocus();
		await user.keyboard("{Delete}");
		expect(onCloseTab).toHaveBeenLastCalledWith("git");
		expect(screen.getByRole("region", { name: "Panel" })).toHaveFocus();
	});

	it("returns focus to the selected tab after activating a close button", async () => {
		const user = userEvent.setup();
		const { onCloseTab } = renderTabs({ activeTabId: "terminal" });
		await user.tab();
		await user.tab();
		await user.keyboard("{Enter}");
		expect(onCloseTab).toHaveBeenCalledWith("terminal");
		expect(
			screen.getByRole("tab", { name: "Preview", selected: true }),
		).toHaveFocus();
		await user.click(screen.getByRole("button", { name: "Close Git tab" }));
		expect(onCloseTab).toHaveBeenLastCalledWith("git");
		expect(
			screen.getByRole("tab", { name: "Preview", selected: true }),
		).toHaveFocus();
	});
});

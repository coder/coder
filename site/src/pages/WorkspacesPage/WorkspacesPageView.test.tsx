import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { API } from "#/api/api";
import type { Workspace } from "#/api/typesGenerated";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	MockDeletingWorkspace,
	MockPendingWorkspace,
	MockStoppedWorkspace,
	MockTemplate,
	MockWorkspace,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { WorkspacesPageView } from "./WorkspacesPageView";

const createFilter = (used = false): UseFilterResult => ({
	query: "",
	values: {},
	used,
	update: vi.fn(),
	debounceUpdate: vi.fn(),
	cancelDebounce: vi.fn(),
});

const defaultProps = {
	error: undefined,
	workspaces: [],
	checkedWorkspaces: [],
	count: 0,
	filter: createFilter(),
	page: 1,
	limit: 25,
	onPageChange: vi.fn(),
	onCheckChange: vi.fn(),
	isRunningBatchAction: false,
	onBatchDeleteTransition: vi.fn(),
	onBatchUpdateTransition: vi.fn(),
	onBatchStartTransition: vi.fn(),
	onBatchStopTransition: vi.fn(),
	templatesFetchStatus: "success" as const,
	templates: [MockTemplate],
	canCreateTemplate: false,
	canCreateWorkspace: true,
	canChangeVersions: false,
	onActionSuccess: vi.fn().mockResolvedValue(undefined),
	onActionError: vi.fn(),
};

describe("WorkspacesPageView", () => {
	it("hides the New workspace button and explains the missing permission", async () => {
		renderWithAuth(
			<WorkspacesPageView {...defaultProps} canCreateWorkspace={false} />,
		);

		await screen.findByText(/don't have permission to create workspaces/i);
		expect(
			screen.queryByRole("button", { name: /new workspace/i }),
		).not.toBeInTheDocument();
	});

	it("shows the New workspace button when the user can create workspaces", async () => {
		renderWithAuth(<WorkspacesPageView {...defaultProps} />);

		expect(
			await screen.findByRole("button", { name: /new workspace/i }),
		).toBeInTheDocument();
	});

	it("shows the filter empty state instead of the no-permission empty state when a filter is active", async () => {
		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				canCreateWorkspace={false}
				filter={createFilter(true)}
			/>,
		);

		await screen.findByRole("heading", {
			name: /no workspaces match your search\./i,
		});
		expect(
			screen.queryByText(/don't have permission to create workspaces/i),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /new workspace/i }),
		).not.toBeInTheDocument();
	});

	it("shows the filter empty state when the user can create workspaces but the filter matches nothing", async () => {
		renderWithAuth(
			<WorkspacesPageView {...defaultProps} filter={createFilter(true)} />,
		);

		await screen.findByRole("heading", {
			name: /no workspaces match your search\./i,
		});
		expect(
			screen.queryByText(/don't have permission to create workspaces/i),
		).not.toBeInTheDocument();
	});

	it("restarts a running workspace from the overflow menu", async () => {
		const user = userEvent.setup();
		const restartSpy = vi
			.spyOn(API, "restartWorkspace")
			.mockResolvedValue(undefined);

		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				workspaces={[MockWorkspace]}
				count={1}
			/>,
		);

		await screen.findByText(MockWorkspace.name);
		await user.click(screen.getByTestId("workspace-options-button"));

		const restartItem = await screen.findByRole("menuitem", {
			name: /restart/i,
		});
		await user.click(restartItem);

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: /restart/i }));

		await waitFor(() =>
			expect(restartSpy).toHaveBeenCalledWith(
				expect.objectContaining({ workspace: MockWorkspace }),
			),
		);
	});

	it("does not offer restart for a stopped workspace", async () => {
		const user = userEvent.setup();

		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				workspaces={[MockStoppedWorkspace]}
				count={1}
			/>,
		);

		await screen.findByText(MockStoppedWorkspace.name);
		await user.click(screen.getByTestId("workspace-options-button"));

		await screen.findByRole("menuitem", { name: /settings/i });
		expect(
			screen.queryByRole("menuitem", { name: /restart/i }),
		).not.toBeInTheDocument();
	});

	describe("drag to select", () => {
		const alpha = { ...MockWorkspace, id: "ws-alpha", name: "alpha" };
		const bravo = { ...MockPendingWorkspace, id: "ws-bravo", name: "bravo" };
		const charlie = {
			...MockStoppedWorkspace,
			id: "ws-charlie",
			name: "charlie",
		};
		const delta = { ...MockWorkspace, id: "ws-delta", name: "delta" };
		const echo = { ...MockDeletingWorkspace, id: "ws-echo", name: "echo" };
		const foxtrot = { ...MockWorkspace, id: "ws-foxtrot", name: "foxtrot" };
		const workspaces = [alpha, bravo, charlie, delta, echo, foxtrot];

		const renderWorkspaces = (
			checkedWorkspaces: readonly Workspace[] = [],
			refetchedWorkspaces?: readonly Workspace[],
		) => {
			const onCheckChange = vi.fn();
			const View = () => {
				const [checked, setChecked] = useState(checkedWorkspaces);
				const [items, setItems] = useState<readonly Workspace[]>(workspaces);
				return (
					<>
						{refetchedWorkspaces && (
							<button
								type="button"
								onClick={() => setItems(refetchedWorkspaces)}
							>
								Refetch workspaces
							</button>
						)}
						<WorkspacesPageView
							{...defaultProps}
							workspaces={items}
							count={items.length}
							checkedWorkspaces={checked}
							onCheckChange={(selection) => {
								setChecked(selection);
								onCheckChange(selection);
							}}
						/>
					</>
				);
			};
			return {
				...renderWithAuth(<View />, {
					path: "/workspaces",
					route: "/workspaces",
					extraRoutes: [{ path: "/@:owner/:workspace", element: null }],
				}),
				onCheckChange,
			};
		};

		const checkboxFor = (workspace: { name: string }) =>
			screen.getByRole("checkbox", {
				name: `Select workspace ${workspace.name}`,
			});

		it("selects only checkable rows, shrinks the range, and preserves selection outside it", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces([foxtrot]);
			await screen.findByText(alpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(alpha) },
				{ target: checkboxFor(bravo) },
				{ target: checkboxFor(charlie) },
				{ target: checkboxFor(delta) },
				{ target: checkboxFor(echo) },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([
				alpha,
				charlie,
				delta,
				foxtrot,
			]);

			await user.pointer([
				{ target: checkboxFor(charlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([alpha, charlie, foxtrot]);
		});

		it("deselects upwards and restores rows when the range shrinks", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces([
				alpha,
				charlie,
				delta,
				foxtrot,
			]);
			await screen.findByText(alpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(delta) },
				{ target: checkboxFor(alpha) },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([foxtrot]);

			await user.pointer([
				{ target: checkboxFor(charlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([alpha, foxtrot]);
		});

		it("preserves focus, single-click selection, and keyboard toggling", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(alpha.name);

			await user.click(checkboxFor(alpha));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([alpha]);
			expect(checkboxFor(alpha)).toHaveFocus();

			await user.keyboard(" ");
			expect(onCheckChange).toHaveBeenCalledTimes(2);
			expect(onCheckChange).toHaveBeenLastCalledWith([]);
		});

		it("does not toggle again when a drag returns to its starting checkbox", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(alpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(alpha) },
				{ target: checkboxFor(delta) },
				{ target: checkboxFor(alpha) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenCalledTimes(2);
			expect(onCheckChange).toHaveBeenLastCalledWith([alpha]);

			await user.click(checkboxFor(charlie));
			expect(onCheckChange).toHaveBeenCalledTimes(3);
			expect(onCheckChange).toHaveBeenLastCalledWith([alpha, charlie]);
		});

		it("does not navigate when a drag ends on its starting row, but allows a subsequent click", async () => {
			const user = userEvent.setup();
			const { onCheckChange, router } = renderWorkspaces();
			const link = await screen.findByRole("link", { name: alpha.name });

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(alpha) },
				{ target: checkboxFor(delta) },
				{ target: link },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([alpha]);
			expect(router.state.location.pathname).toBe("/workspaces");

			await user.click(link);
			expect(router.state.location.pathname).toBe(
				`/@${alpha.owner_name}/${alpha.name}`,
			);
		});

		it("stops selecting after releasing outside the table", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(alpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(alpha) },
				{ target: checkboxFor(charlie) },
				{ keys: "[/MouseLeft]", target: document.body },
			]);
			await user.hover(checkboxFor(delta));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([alpha, charlie]);
		});

		it.each(["pointercancel", "blur"])(
			"stops selecting after %s",
			async (event) => {
				const user = userEvent.setup();
				const { onCheckChange } = renderWorkspaces();
				await screen.findByText(alpha.name);

				await user.pointer([
					{ keys: "[MouseLeft>]", target: checkboxFor(alpha) },
					{ target: checkboxFor(charlie) },
				]);
				if (event === "pointercancel") {
					fireEvent.pointerCancel(window, { pointerId: 1 });
				} else {
					fireEvent(window, new Event("blur"));
				}
				await user.pointer([
					{ target: checkboxFor(delta) },
					{ keys: "[/MouseLeft]", target: document.body },
				]);
				expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([alpha, charlie]);
			},
		);

		it("does not keep selecting when a pointer release was missed", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(alpha.name);
			await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor(alpha) });

			const releasedPointer = userEvent.setup();
			await releasedPointer.hover(checkboxFor(charlie));
			expect(onCheckChange).not.toHaveBeenCalled();

			await releasedPointer.click(checkboxFor(delta));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([delta]);
		});

		it.each(["MouseRight", "MouseMiddle", "TouchA"])(
			"does not start a drag with %s",
			async (pointerName) => {
				const user = userEvent.setup();
				const { onCheckChange } = renderWorkspaces();
				await screen.findByText(alpha.name);

				await user.pointer([
					{ keys: `[${pointerName}>]`, target: checkboxFor(alpha) },
					{
						pointerName: pointerName === "TouchA" ? "TouchA" : "mouse",
						target: checkboxFor(delta),
					},
				]);
				expect(onCheckChange).not.toHaveBeenCalled();
				await user.pointer({
					keys: `[/${pointerName}]`,
					target: document.body,
				});
			},
		);

		it("does not start a drag outside a checkbox", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			const link = await screen.findByRole("link", { name: alpha.name });

			await user.pointer([
				{ keys: "[MouseLeft>]", target: link },
				{ target: checkboxFor(delta) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).not.toHaveBeenCalled();
		});

		it("keeps the same anchor workspace when a refetch reorders the rows", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces(
				[],
				[delta, alpha, bravo, charlie],
			);
			await screen.findByText(alpha.name);
			await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor(alpha) });

			fireEvent.click(
				screen.getByRole("button", { name: "Refetch workspaces" }),
			);
			await user.pointer([
				{ target: checkboxFor(charlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([alpha, charlie]);
		});

		it("stops selecting when a refetch removes the anchor workspace", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces([], [bravo, charlie, delta]);
			await screen.findByText(alpha.name);
			await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor(alpha) });

			fireEvent.click(
				screen.getByRole("button", { name: "Refetch workspaces" }),
			);
			await user.pointer([
				{ target: checkboxFor(charlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).not.toHaveBeenCalled();
		});
	});
});

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

const stoppedWorkspace = { ...MockStoppedWorkspace, name: "stopped" };
const pendingWorkspace = { ...MockPendingWorkspace, name: "pending" };
const runningWorkspace = { ...MockWorkspace, name: "running" };
const deletingWorkspace = { ...MockDeletingWorkspace, name: "deleting" };

function renderSelection(
	initialWorkspaces: readonly Workspace[],
	refreshedWorkspaces?: readonly Workspace[],
) {
	const onCheckChange = vi.fn();
	const View = () => {
		const [workspaces, setWorkspaces] = useState(initialWorkspaces);
		const [checkedWorkspaces, setCheckedWorkspaces] = useState<
			readonly Workspace[]
		>([]);

		return (
			<>
				{refreshedWorkspaces && (
					<button
						type="button"
						onClick={() => setWorkspaces(refreshedWorkspaces)}
					>
						Refresh workspaces
					</button>
				)}
				<WorkspacesPageView
					{...defaultProps}
					workspaces={workspaces}
					count={workspaces.length}
					checkedWorkspaces={workspaces.filter((workspace) =>
						checkedWorkspaces.some((selected) => selected.id === workspace.id),
					)}
					onCheckChange={(selected) => {
						onCheckChange(selected);
						setCheckedWorkspaces(selected);
					}}
				/>
			</>
		);
	};

	renderWithAuth(<View />);
	return onCheckChange;
}

describe("WorkspacesPageView", () => {
	it.each([
		[
			"mixed eligibility",
			[stoppedWorkspace, pendingWorkspace, runningWorkspace, deletingWorkspace],
		],
		["all eligible", [stoppedWorkspace, runningWorkspace]],
	])(
		"selects and deselects eligible workspaces with %s",
		async (_name, workspaces) => {
			const user = userEvent.setup();
			const onCheckChange = renderSelection(workspaces);
			const selectAll = await screen.findByRole("checkbox", {
				name: "Select all workspaces",
			});

			await user.click(
				screen.getByRole("checkbox", { name: "Select workspace stopped" }),
			);
			expect(onCheckChange).toHaveBeenLastCalledWith([stoppedWorkspace]);

			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenLastCalledWith([
				stoppedWorkspace,
				runningWorkspace,
			]);

			await user.click(
				screen.getByRole("checkbox", { name: "Select workspace running" }),
			);
			expect(onCheckChange).toHaveBeenLastCalledWith([stoppedWorkspace]);

			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenLastCalledWith([
				stoppedWorkspace,
				runningWorkspace,
			]);

			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenLastCalledWith([]);
		},
	);

	it.each([pendingWorkspace, deletingWorkspace])(
		"clears a selected workspace after it becomes $name on refresh",
		async (disabledWorkspace) => {
			const user = userEvent.setup();
			const onCheckChange = renderSelection(
				[runningWorkspace],
				[
					{
						...disabledWorkspace,
						id: runningWorkspace.id,
						name: runningWorkspace.name,
					},
				],
			);
			const selectAll = await screen.findByRole("checkbox", {
				name: "Select all workspaces",
			});

			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenLastCalledWith([runningWorkspace]);
			await user.click(
				screen.getByRole("button", { name: "Refresh workspaces" }),
			);
			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenLastCalledWith([]);
			expect(onCheckChange).toHaveBeenCalledTimes(2);

			await user.click(selectAll);
			expect(onCheckChange).toHaveBeenCalledTimes(2);
		},
	);

	it("does not select workspaces when no rows are eligible", async () => {
		const user = userEvent.setup();
		const onCheckChange = renderSelection([
			pendingWorkspace,
			deletingWorkspace,
		]);

		await user.click(
			await screen.findByRole("checkbox", { name: "Select all workspaces" }),
		);
		await user.click(
			screen.getByRole("checkbox", { name: "Select workspace pending" }),
		);
		await user.click(
			screen.getByRole("checkbox", { name: "Select workspace deleting" }),
		);

		expect(onCheckChange).not.toHaveBeenCalled();
	});

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
		const mockAlpha = { ...MockWorkspace, id: "ws-alpha", name: "alpha" };
		const mockBravo = {
			...MockPendingWorkspace,
			id: "ws-bravo",
			name: "bravo",
		};
		const mockCharlie = {
			...MockStoppedWorkspace,
			id: "ws-charlie",
			name: "charlie",
		};
		const mockDelta = { ...MockWorkspace, id: "ws-delta", name: "delta" };
		const mockEcho = { ...MockDeletingWorkspace, id: "ws-echo", name: "echo" };
		const mockFoxtrot = { ...MockWorkspace, id: "ws-foxtrot", name: "foxtrot" };
		const workspaces = [
			mockAlpha,
			mockBravo,
			mockCharlie,
			mockDelta,
			mockEcho,
			mockFoxtrot,
		];

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
			const { onCheckChange } = renderWorkspaces([mockFoxtrot]);
			await screen.findByText(mockAlpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(mockAlpha) },
				{ target: checkboxFor(mockBravo) },
				{ target: checkboxFor(mockCharlie) },
				{ target: checkboxFor(mockDelta) },
				{ target: checkboxFor(mockEcho) },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([
				mockAlpha,
				mockCharlie,
				mockDelta,
				mockFoxtrot,
			]);

			await user.pointer([
				{ target: checkboxFor(mockCharlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([
				mockAlpha,
				mockCharlie,
				mockFoxtrot,
			]);
		});

		it("deselects upwards and restores rows when the range shrinks", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces([
				mockAlpha,
				mockCharlie,
				mockDelta,
				mockFoxtrot,
			]);
			await screen.findByText(mockAlpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(mockDelta) },
				{ target: checkboxFor(mockAlpha) },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([mockFoxtrot]);

			await user.pointer([
				{ target: checkboxFor(mockCharlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([mockAlpha, mockFoxtrot]);
		});

		it("preserves focus, single-click selection, and keyboard toggling", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(mockAlpha.name);

			await user.click(checkboxFor(mockAlpha));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([mockAlpha]);
			expect(checkboxFor(mockAlpha)).toHaveFocus();

			await user.keyboard(" ");
			expect(onCheckChange).toHaveBeenCalledTimes(2);
			expect(onCheckChange).toHaveBeenLastCalledWith([]);
		});

		it("does not toggle again when a drag returns to its starting checkbox", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(mockAlpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(mockAlpha) },
				{ target: checkboxFor(mockDelta) },
				{ target: checkboxFor(mockAlpha) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenCalledTimes(2);
			expect(onCheckChange).toHaveBeenLastCalledWith([mockAlpha]);

			await user.click(checkboxFor(mockCharlie));
			expect(onCheckChange).toHaveBeenCalledTimes(3);
			expect(onCheckChange).toHaveBeenLastCalledWith([mockAlpha, mockCharlie]);
		});

		it("does not navigate when a drag ends on its starting row, but allows a subsequent click", async () => {
			const user = userEvent.setup();
			const { onCheckChange, router } = renderWorkspaces();
			const link = await screen.findByRole("link", { name: mockAlpha.name });

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(mockAlpha) },
				{ target: checkboxFor(mockDelta) },
				{ target: link },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenLastCalledWith([mockAlpha]);
			expect(router.state.location.pathname).toBe("/workspaces");

			await user.click(link);
			expect(router.state.location.pathname).toBe(
				`/@${mockAlpha.owner_name}/${mockAlpha.name}`,
			);
		});

		it("stops selecting after releasing outside the table", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(mockAlpha.name);

			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor(mockAlpha) },
				{ target: checkboxFor(mockCharlie) },
				{ keys: "[/MouseLeft]", target: document.body },
			]);
			await user.hover(checkboxFor(mockDelta));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([
				mockAlpha,
				mockCharlie,
			]);
		});

		it.each(["pointercancel", "blur"])(
			"stops selecting after %s",
			async (event) => {
				const user = userEvent.setup();
				const { onCheckChange } = renderWorkspaces();
				await screen.findByText(mockAlpha.name);

				await user.pointer([
					{ keys: "[MouseLeft>]", target: checkboxFor(mockAlpha) },
					{ target: checkboxFor(mockCharlie) },
				]);
				if (event === "pointercancel") {
					fireEvent.pointerCancel(window, { pointerId: 1 });
				} else {
					fireEvent(window, new Event("blur"));
				}
				await user.pointer([
					{ target: checkboxFor(mockDelta) },
					{ keys: "[/MouseLeft]", target: document.body },
				]);
				expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([
					mockAlpha,
					mockCharlie,
				]);
			},
		);

		it("does not keep selecting when a pointer release was missed", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces();
			await screen.findByText(mockAlpha.name);
			await user.pointer({
				keys: "[MouseLeft>]",
				target: checkboxFor(mockAlpha),
			});

			const releasedPointer = userEvent.setup();
			await releasedPointer.hover(checkboxFor(mockCharlie));
			expect(onCheckChange).not.toHaveBeenCalled();

			await releasedPointer.click(checkboxFor(mockDelta));
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([mockDelta]);
		});

		it.each(["MouseRight", "MouseMiddle", "TouchA"])(
			"does not start a drag with %s",
			async (pointerName) => {
				const user = userEvent.setup();
				const { onCheckChange } = renderWorkspaces();
				await screen.findByText(mockAlpha.name);

				await user.pointer([
					{ keys: `[${pointerName}>]`, target: checkboxFor(mockAlpha) },
					{
						pointerName: pointerName === "TouchA" ? "TouchA" : "mouse",
						target: checkboxFor(mockDelta),
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
			const link = await screen.findByRole("link", { name: mockAlpha.name });

			await user.pointer([
				{ keys: "[MouseLeft>]", target: link },
				{ target: checkboxFor(mockDelta) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).not.toHaveBeenCalled();
		});

		it("keeps the same anchor workspace when a refetch reorders the rows", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces(
				[],
				[mockDelta, mockAlpha, mockBravo, mockCharlie],
			);
			await screen.findByText(mockAlpha.name);
			await user.pointer({
				keys: "[MouseLeft>]",
				target: checkboxFor(mockAlpha),
			});

			fireEvent.click(
				screen.getByRole("button", { name: "Refetch workspaces" }),
			);
			await user.pointer([
				{ target: checkboxFor(mockCharlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).toHaveBeenCalledExactlyOnceWith([
				mockAlpha,
				mockCharlie,
			]);
		});

		it("stops selecting when a refetch removes the anchor workspace", async () => {
			const user = userEvent.setup();
			const { onCheckChange } = renderWorkspaces(
				[],
				[mockBravo, mockCharlie, mockDelta],
			);
			await screen.findByText(mockAlpha.name);
			await user.pointer({
				keys: "[MouseLeft>]",
				target: checkboxFor(mockAlpha),
			});

			fireEvent.click(
				screen.getByRole("button", { name: "Refetch workspaces" }),
			);
			await user.pointer([
				{ target: checkboxFor(mockCharlie) },
				{ keys: "[/MouseLeft]" },
			]);
			expect(onCheckChange).not.toHaveBeenCalled();
		});
	});
});

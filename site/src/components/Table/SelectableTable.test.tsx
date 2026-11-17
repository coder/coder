import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	SelectableTable,
	SelectableTableCheckbox,
	SelectableTableRow,
	SelectableTableSelectAll,
} from "./SelectableTable";
import {
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./Table";

type Row = {
	id: string;
	disabled?: boolean;
};

const rows = [
	{ id: "alpha" },
	{ id: "bravo", disabled: true },
	{ id: "charlie" },
	{ id: "delta" },
	{ id: "echo", disabled: true },
	{ id: "foxtrot" },
] satisfies Row[];

function renderSelection({
	initialRows = rows,
	initialIds = [],
	refetchedRows,
	refetchedIds,
	onRowClick,
	onCheckboxClick,
	onCheckboxPointerDown,
	onRowPointerEnter,
}: {
	initialRows?: readonly Row[];
	initialIds?: readonly string[];
	refetchedRows?: readonly Row[];
	refetchedIds?: readonly string[];
	onRowClick?: () => void;
	onCheckboxClick?: () => void;
	onCheckboxPointerDown?: () => void;
	onRowPointerEnter?: () => void;
} = {}) {
	const onSelectionChange = vi.fn();
	function View() {
		const [displayRows, setDisplayRows] = useState(initialRows);
		const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(
			new Set(initialIds),
		);
		return (
			<>
				{refetchedRows && (
					<button
						type="button"
						onClick={() => {
							setDisplayRows(refetchedRows);
							if (refetchedIds) {
								setSelectedIds(new Set(refetchedIds));
							}
						}}
					>
						Refresh rows
					</button>
				)}
				<SelectableTable
					aria-label="Selectable examples"
					rows={displayRows}
					selectedIds={selectedIds}
					onSelectionChange={(ids) => {
						onSelectionChange(ids);
						setSelectedIds(ids);
					}}
				>
					<TableHeader>
						<TableRow>
							<TableHead>
								<SelectableTableSelectAll aria-label="Select all examples" />
								Examples
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{displayRows.map((row) => (
							<SelectableTableRow
								key={row.id}
								rowId={row.id}
								onClick={onRowClick}
								onPointerEnter={onRowPointerEnter}
							>
								<TableCell>
									<SelectableTableCheckbox
										aria-label={`Select ${row.id}`}
										onClick={onCheckboxClick}
										onPointerDown={onCheckboxPointerDown}
									/>
									{row.id}
								</TableCell>
							</SelectableTableRow>
						))}
					</TableBody>
				</SelectableTable>
			</>
		);
	}
	renderComponent(<View />);
	return onSelectionChange;
}

const checkboxFor = (id: string) =>
	screen.getByRole("checkbox", { name: `Select ${id}` });

describe("SelectableTable", () => {
	it("toggles a row once by click and Space without activating the row", async () => {
		const user = userEvent.setup();
		const onRowClick = vi.fn();
		const onSelectionChange = renderSelection({ onRowClick });
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha"]),
		);
		expect(onRowClick).not.toHaveBeenCalled();
		await user.keyboard(" ");
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
		expect(onSelectionChange).toHaveBeenCalledTimes(2);
		expect(onRowClick).not.toHaveBeenCalled();
	});

	it.each(["Shift-click", "Shift+Space"])(
		"selects, shrinks, and deselects anchored ranges with %s without activating rows",
		async (gesture) => {
			const user = userEvent.setup();
			const onRowClick = vi.fn();
			const onSelectionChange = renderSelection({
				initialIds: ["foxtrot", "stale"],
				onRowClick,
			});
			await user.click(checkboxFor("alpha"));
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("delta"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab();
				await user.tab();
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "delta", "foxtrot", "stale"]),
			);
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("charlie"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab({ shift: true });
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "foxtrot", "stale"]),
			);
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("delta"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab();
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "delta", "foxtrot", "stale"]),
			);
			await user.click(checkboxFor("delta"));
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("alpha"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab({ shift: true });
				await user.tab({ shift: true });
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["foxtrot", "stale"]),
			);
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("charlie"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab();
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "foxtrot", "stale"]),
			);
			if (gesture === "Shift-click") {
				await user.keyboard("{Shift>}");
				await user.click(checkboxFor("delta"));
				await user.keyboard("{/Shift}");
			} else {
				await user.tab();
				await user.keyboard("{Shift>} {/Shift}");
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "foxtrot", "stale"]),
			);
			expect(onSelectionChange).toHaveBeenCalledTimes(8);
			expect(onRowClick).not.toHaveBeenCalled();
		},
	);

	it("toggles a single row when Shift-click has no anchor or targets the anchor", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha"]),
		);
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
		expect(onSelectionChange).toHaveBeenCalledTimes(2);
	});

	it.each([
		{
			name: "removed",
			refetchedRows: rows.slice(1),
			refetchedIds: undefined,
			expectedIds: ["alpha", "delta"],
		},
		{
			name: "disabled",
			refetchedRows: [{ ...rows[0], disabled: true }, ...rows.slice(1)],
			refetchedIds: undefined,
			expectedIds: ["alpha", "delta"],
		},
		{
			name: "cleared externally",
			refetchedRows: rows,
			refetchedIds: [],
			expectedIds: ["charlie"],
		},
		{
			name: "preserved but selection replaced externally",
			refetchedRows: rows,
			refetchedIds: ["alpha", "foxtrot"],
			expectedIds: ["alpha", "charlie", "foxtrot"],
		},
	])(
		"toggles only the clicked row when the range anchor is $name",
		async ({ refetchedRows, refetchedIds, expectedIds }) => {
			const user = userEvent.setup();
			const onSelectionChange = renderSelection({
				refetchedRows,
				refetchedIds,
			});
			await user.click(checkboxFor("alpha"));
			await user.keyboard("{Shift>}");
			await user.click(checkboxFor("delta"));
			await user.keyboard("{/Shift}");
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "delta"]),
			);
			await user.click(screen.getByRole("button", { name: "Refresh rows" }));
			await user.keyboard("{Shift>}");
			await user.click(checkboxFor("charlie"));
			expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(expectedIds));
		},
	);

	it("resets the range anchor after selecting all rows", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.click(checkboxFor("alpha"));
		await user.click(
			screen.getByRole("checkbox", { name: "Select all examples" }),
		);
		await user.keyboard("{Shift>}");
		await user.click(checkboxFor("charlie"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "delta", "foxtrot"]),
		);
	});

	it("clears disabled and stale selections when all eligible IDs are selected", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			initialIds: ["alpha", "charlie", "delta", "foxtrot", "bravo", "stale"],
		});
		const selectAll = screen.getByRole("checkbox", {
			name: "Select all examples",
		});
		await user.click(selectAll);
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
		await user.click(selectAll);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta", "foxtrot"]),
		);
	});

	it("clears selected IDs even when there are no eligible rows", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			initialRows: [rows[1], rows[4]],
			initialIds: ["bravo", "stale"],
		});
		await user.click(
			screen.getByRole("checkbox", { name: "Select all examples" }),
		);
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(new Set());
	});

	it("ignores disabled rows on click and drag without losing off-range or stale IDs", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			initialIds: ["foxtrot", "stale"],
		});
		await user.click(checkboxFor("bravo"));
		expect(onSelectionChange).not.toHaveBeenCalled();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("bravo") },
			{ target: checkboxFor("charlie") },
			{ target: checkboxFor("delta") },
			{ target: checkboxFor("echo") },
		]);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta", "foxtrot", "stale"]),
		);
		await user.pointer([
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "foxtrot", "stale"]),
		);
	});

	it("deselects upwards and restores the initial snapshot on shrink", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			initialIds: ["alpha", "charlie", "delta", "foxtrot"],
		});
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("delta") },
			{ target: checkboxFor("alpha") },
		]);
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["foxtrot"]));
		await user.pointer([
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "foxtrot"]),
		);
	});

	it("suppresses the terminal drag click but permits the next click", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("delta") },
			{ target: checkboxFor("alpha") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenCalledTimes(2);
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
		await user.click(checkboxFor("foxtrot"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta", "foxtrot"]),
		);
		await user.keyboard("{/Shift}");
		await user.click(checkboxFor("charlie"));
		expect(onSelectionChange).toHaveBeenCalledTimes(4);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "delta", "foxtrot"]),
		);
	});

	it("stops extending without Shift while still suppressing the terminal drag click", async () => {
		const user = userEvent.setup();
		const onRowClick = vi.fn();
		const onSelectionChange = renderSelection({ onRowClick });
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("charlie") },
		]);
		await user.keyboard("{/Shift}");
		await user.pointer([
			{ target: checkboxFor("delta") },
			{ target: checkboxFor("alpha") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha", "charlie"]),
		);
		expect(onRowClick).not.toHaveBeenCalled();
	});

	it("ends a drag on pointerup outside the table", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		await user.pointer([
			{ keys: "[MouseLeft>]", target: document.body },
			{ target: checkboxFor("delta") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha", "charlie"]),
		);
	});

	it.each(["pointercancel", "blur"])("ends a drag on %s", async (event) => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("charlie") },
		]);
		if (event === "pointercancel") {
			fireEvent.pointerCancel(window, { pointerId: 1 });
		} else {
			fireEvent(window, new Event("blur"));
		}
		await user.pointer([
			{ target: checkboxFor("delta") },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha", "charlie"]),
		);
	});

	it("ignores a gesture if the mouse button is no longer held", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor("alpha") });
		const releasedPointer = userEvent.setup();
		await releasedPointer.hover(checkboxFor("charlie"));
		expect(onSelectionChange).not.toHaveBeenCalled();
		await releasedPointer.click(checkboxFor("delta"));
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["delta"]),
		);
	});

	it.each([
		{ pointerName: "MouseLeft", shift: false },
		{ pointerName: "MouseRight", shift: true },
		{ pointerName: "MouseMiddle", shift: true },
		{ pointerName: "TouchA", shift: true },
	])(
		"does not start a drag with $pointerName and Shift=$shift",
		async ({ pointerName, shift }) => {
			const user = userEvent.setup();
			const onSelectionChange = renderSelection();
			if (shift) {
				await user.keyboard("{Shift>}");
			}
			await user.pointer([
				{ keys: `[${pointerName}>]`, target: checkboxFor("alpha") },
				{
					pointerName: pointerName === "TouchA" ? "TouchA" : "mouse",
					target: checkboxFor("delta"),
				},
			]);
			expect(onSelectionChange).not.toHaveBeenCalled();
			await user.pointer({ keys: `[/${pointerName}]`, target: document.body });
		},
	);

	it("does not start a drag from elsewhere in the row", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: screen.getByText("alpha") },
			{ target: checkboxFor("delta") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).not.toHaveBeenCalled();
	});

	it.each(["Shift-click", "Shift-drag"])(
		"tracks the anchor by ID when rows reorder before extending with %s",
		async (gesture) => {
			const user = userEvent.setup();
			const onSelectionChange = renderSelection({
				refetchedRows: [rows[3], rows[0], rows[1], rows[2], rows[5]],
			});
			if (gesture === "Shift-click") {
				await user.click(checkboxFor("alpha"));
				await user.keyboard("{Shift>}");
			} else {
				await user.keyboard("{Shift>}");
				await user.pointer({
					keys: "[MouseLeft>]",
					target: checkboxFor("alpha"),
				});
			}
			fireEvent.click(screen.getByRole("button", { name: "Refresh rows" }));
			if (gesture === "Shift-click") {
				await user.click(checkboxFor("foxtrot"));
			} else {
				await user.pointer([
					{ target: checkboxFor("foxtrot") },
					{ keys: "[/MouseLeft]" },
				]);
			}
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "foxtrot"]),
			);
		},
	);

	it.each([
		{
			name: "removes its anchor",
			refetchedRows: rows.slice(1),
			refetchedIds: undefined,
		},
		{
			name: "disables its anchor",
			refetchedRows: [{ ...rows[0], disabled: true }, ...rows.slice(1)],
			refetchedIds: undefined,
		},
		{ name: "clears selection", refetchedRows: rows, refetchedIds: [] },
		{
			name: "replaces selection",
			refetchedRows: rows,
			refetchedIds: ["delta", "foxtrot"],
		},
	])(
		"cancels a drag when refetch $name",
		async ({ refetchedRows, refetchedIds }) => {
			const user = userEvent.setup();
			const onRowClick = vi.fn();
			const onSelectionChange = renderSelection({
				refetchedRows,
				refetchedIds,
				onRowClick,
			});
			await user.keyboard("{Shift>}");
			await user.pointer([
				{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
				{ target: checkboxFor("charlie") },
			]);
			fireEvent.click(screen.getByRole("button", { name: "Refresh rows" }));
			await user.pointer([
				{ target: checkboxFor("delta") },
				{
					target: refetchedRows.some((row) => row.id === "alpha")
						? screen.getByText("alpha")
						: checkboxFor("charlie"),
				},
				{ keys: "[/MouseLeft]" },
			]);
			expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
				new Set(["alpha", "charlie"]),
			);
			expect(onRowClick).not.toHaveBeenCalled();
		},
	);

	it("keeps extending before the parent commits a selection update", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor("alpha") });
		await act(async () => {
			for (const id of ["charlie", "delta"]) {
				fireEvent.pointerOver(checkboxFor(id), {
					pointerId: 1,
					pointerType: "mouse",
					buttons: 1,
					shiftKey: true,
				});
			}
		});
		await user.pointer({ keys: "[/MouseLeft]", target: document.body });
		expect(onSelectionChange).toHaveBeenCalledTimes(2);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta"]),
		);
	});

	it("keeps selection and gestures isolated across two tables", async () => {
		const user = userEvent.setup();
		const onFirstChange = vi.fn();
		const onSecondChange = vi.fn();
		function TwoTables() {
			const [first, setFirst] = useState<ReadonlySet<string>>(new Set());
			const [second, setSecond] = useState<ReadonlySet<string>>(new Set());
			return (
				<>
					{[
						{
							name: "First",
							selectedIds: first,
							update: setFirst,
							callback: onFirstChange,
						},
						{
							name: "Second",
							selectedIds: second,
							update: setSecond,
							callback: onSecondChange,
						},
					].map(({ name, selectedIds, update, callback }) => (
						<SelectableTable
							key={name}
							aria-label={name}
							rows={[{ id: "same" }, { id: "other" }]}
							selectedIds={selectedIds}
							onSelectionChange={(ids) => {
								callback(ids);
								update(ids);
							}}
						>
							<TableBody>
								{["same", "other"].map((id) => (
									<SelectableTableRow rowId={id} key={id}>
										<TableCell>
											<SelectableTableCheckbox
												aria-label={`Select ${name} ${id}`}
											/>
										</TableCell>
									</SelectableTableRow>
								))}
							</TableBody>
						</SelectableTable>
					))}
				</>
			);
		}
		renderComponent(<TwoTables />);
		await user.keyboard("{Shift>}");
		await user.pointer([
			{
				keys: "[MouseLeft>]",
				target: screen.getByRole("checkbox", { name: "Select First same" }),
			},
			{ target: screen.getByRole("checkbox", { name: "Select Second other" }) },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		expect(onFirstChange).not.toHaveBeenCalled();
		expect(onSecondChange).not.toHaveBeenCalled();
		await user.keyboard("{/Shift}");
		await user.click(
			screen.getByRole("checkbox", { name: "Select Second same" }),
		);
		expect(onSecondChange).toHaveBeenCalledExactlyOnceWith(new Set(["same"]));
		await user.click(
			screen.getByRole("checkbox", { name: "Select First other" }),
		);
		expect(onFirstChange).toHaveBeenCalledExactlyOnceWith(new Set(["other"]));
		expect(onSecondChange).toHaveBeenCalledTimes(1);
	});

	it("forwards table, row, and checkbox refs to their native elements", () => {
		const tableRef = createRef<HTMLTableElement>();
		const rowRef = createRef<HTMLTableRowElement>();
		const checkboxRef = createRef<HTMLButtonElement>();
		const selectAllRef = createRef<HTMLButtonElement>();
		renderComponent(
			<SelectableTable
				aria-label="Ref examples"
				ref={tableRef}
				rows={[{ id: "one" }]}
				selectedIds={new Set()}
				onSelectionChange={vi.fn()}
			>
				<TableHeader>
					<TableRow>
						<TableHead>
							<SelectableTableSelectAll
								ref={selectAllRef}
								aria-label="Select all refs"
							/>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<SelectableTableRow ref={rowRef} rowId="one">
						<TableCell>
							<SelectableTableCheckbox
								ref={checkboxRef}
								aria-label="Select one"
							/>
							One
						</TableCell>
					</SelectableTableRow>
				</TableBody>
			</SelectableTable>,
		);
		expect(tableRef.current).toBe(
			screen.getByRole("table", { name: "Ref examples" }),
		);
		expect(rowRef.current).toBe(screen.getByRole("row", { name: /One/ }));
		expect(checkboxRef.current).toBe(
			screen.getByRole("checkbox", { name: "Select one" }),
		);
		expect(selectAllRef.current).toBe(
			screen.getByRole("checkbox", { name: "Select all refs" }),
		);
	});

	it("invokes consumer event handlers while retaining selection behavior", async () => {
		const user = userEvent.setup();
		const onCheckboxClick = vi.fn();
		const onCheckboxPointerDown = vi.fn();
		const onRowPointerEnter = vi.fn();
		const onSelectionChange = renderSelection({
			onCheckboxClick,
			onCheckboxPointerDown,
			onRowPointerEnter,
		});
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		expect(onCheckboxPointerDown).toHaveBeenCalled();
		expect(onRowPointerEnter).toHaveBeenCalled();
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie"]),
		);
		await user.keyboard("{/Shift}");
		await user.click(checkboxFor("delta"));
		expect(onCheckboxClick).toHaveBeenCalled();
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta"]),
		);
	});
});

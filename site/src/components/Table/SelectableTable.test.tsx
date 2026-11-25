import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
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
	renderedRows,
	initialIds = [],
	refetchedRows,
	refetchedIds,
	onRowClick,
}: {
	initialRows?: readonly Row[];
	renderedRows?: readonly Row[];
	initialIds?: readonly string[];
	refetchedRows?: readonly Row[];
	refetchedIds?: readonly string[];
	onRowClick?: () => void;
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
						{(renderedRows ?? displayRows).map((row) => (
							<SelectableTableRow
								key={row.id}
								rowId={row.id}
								onClick={onRowClick}
							>
								<TableCell>
									<SelectableTableCheckbox aria-label={`Select ${row.id}`} />
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
	it("toggles a row by click and Space without activating the row", async () => {
		const user = userEvent.setup();
		const onRowClick = vi.fn();
		const onSelectionChange = renderSelection({ onRowClick });
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
		expect(onRowClick).not.toHaveBeenCalled();
		await user.keyboard(" ");
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
		expect(onRowClick).not.toHaveBeenCalled();
	});

	it.each(["Shift-click", "Shift+Space"])(
		"selects, shrinks, and deselects anchored ranges with %s without activating rows",
		async (gesture) => {
			const user = userEvent.setup();
			const extendRangeTo = async (id: string, tabOffset: number) => {
				if (gesture === "Shift-click") {
					await user.keyboard("{Shift>}");
					await user.click(checkboxFor(id));
					await user.keyboard("{/Shift}");
					return;
				}
				for (let i = 0; i < Math.abs(tabOffset); i++) {
					await user.tab({ shift: tabOffset < 0 });
				}
				await user.keyboard("{Shift>} {/Shift}");
			};
			const onRowClick = vi.fn();
			const onSelectionChange = renderSelection({
				initialIds: ["foxtrot", "stale"],
				onRowClick,
			});
			await user.click(checkboxFor("alpha"));
			await extendRangeTo("delta", 2);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "delta", "foxtrot", "stale"]),
			);
			await extendRangeTo("charlie", -1);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "foxtrot", "stale"]),
			);
			await extendRangeTo("delta", 1);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "delta", "foxtrot", "stale"]),
			);
			await user.click(checkboxFor("delta"));
			await extendRangeTo("alpha", -2);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["foxtrot", "stale"]),
			);
			await extendRangeTo("charlie", 1);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "foxtrot", "stale"]),
			);
			await extendRangeTo("delta", 1);
			expect(onSelectionChange).toHaveBeenLastCalledWith(
				new Set(["alpha", "charlie", "foxtrot", "stale"]),
			);
			expect(onRowClick).not.toHaveBeenCalled();
		},
	);

	it("toggles a single row when Shift-click has no anchor or targets the anchor", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.keyboard("{Shift>}");
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
	});

	it.each([
		{
			name: "disabled",
			refetchedRows: [{ ...rows[0], disabled: true }, ...rows.slice(1)],
			refetchedIds: undefined,
			expectedIds: ["alpha", "delta"],
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set());
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

	it("ignores rows missing from the table data until they are registered", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			initialRows: rows.slice(1),
			renderedRows: rows,
			refetchedRows: rows,
		});
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).not.toHaveBeenCalled();
		await user.keyboard("{Shift>}");
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("charlie") },
			{ target: checkboxFor("alpha") },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		expect(onSelectionChange).not.toHaveBeenCalled();
		await user.keyboard("{/Shift}");
		await user.click(screen.getByRole("button", { name: "Refresh rows" }));
		await user.click(checkboxFor("alpha"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
		await user.click(checkboxFor("foxtrot"));
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta", "foxtrot"]),
		);
		await user.keyboard("{/Shift}");
		await user.click(checkboxFor("charlie"));
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["delta"]));
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
		{ name: "clears selection", refetchedRows: rows, refetchedIds: [] },
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
			expect(onSelectionChange).toHaveBeenLastCalledWith(
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
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta"]),
		);
	});
});

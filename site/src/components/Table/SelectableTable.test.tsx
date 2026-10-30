import { fireEvent, screen } from "@testing-library/react";
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

const rows = [
	{ id: "alpha" },
	{ id: "bravo", disabled: true },
	{ id: "charlie" },
	{ id: "delta" },
	{ id: "echo", disabled: true },
	{ id: "foxtrot" },
];

type Row = (typeof rows)[number];

function renderSelection({
	initialRows = rows,
	initialIds = [],
	refetchedRows,
	onRowClick,
	onCheckboxClick,
	onCheckboxPointerDown,
	onRowPointerEnter,
}: {
	initialRows?: readonly Row[];
	initialIds?: readonly string[];
	refetchedRows?: readonly Row[];
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
					<button type="button" onClick={() => setDisplayRows(refetchedRows)}>
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
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("delta") },
			{ target: checkboxFor("alpha") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenCalledTimes(2);
		expect(onSelectionChange).toHaveBeenLastCalledWith(new Set(["alpha"]));
		await user.click(checkboxFor("charlie"));
		expect(onSelectionChange).toHaveBeenCalledTimes(3);
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie"]),
		);
	});

	it("ends a drag on pointerup outside the table", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
		await user.pointer([
			{ keys: "[MouseLeft>]", target: checkboxFor("alpha") },
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]", target: document.body },
		]);
		await user.hover(checkboxFor("delta"));
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha", "charlie"]),
		);
	});

	it.each(["pointercancel", "blur"])("ends a drag on %s", async (event) => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection();
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
		await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor("alpha") });
		const releasedPointer = userEvent.setup();
		await releasedPointer.hover(checkboxFor("charlie"));
		expect(onSelectionChange).not.toHaveBeenCalled();
		await releasedPointer.click(checkboxFor("delta"));
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["delta"]),
		);
	});

	it.each(["MouseRight", "MouseMiddle", "TouchA"])(
		"does not start a drag with %s",
		async (pointerName) => {
			const user = userEvent.setup();
			const onSelectionChange = renderSelection();
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
		await user.pointer([
			{ keys: "[MouseLeft>]", target: screen.getByText("alpha") },
			{ target: checkboxFor("delta") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).not.toHaveBeenCalled();
	});

	it("tracks the anchor by ID when rows reorder during a drag", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			refetchedRows: [rows[3], rows[0], rows[1], rows[2]],
		});
		await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor("alpha") });
		fireEvent.click(screen.getByRole("button", { name: "Refresh rows" }));
		await user.pointer([
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith(
			new Set(["alpha", "charlie"]),
		);
	});

	it("cancels a drag when refetch removes its anchor", async () => {
		const user = userEvent.setup();
		const onSelectionChange = renderSelection({
			refetchedRows: [rows[1], rows[2], rows[3]],
		});
		await user.pointer({ keys: "[MouseLeft>]", target: checkboxFor("alpha") });
		fireEvent.click(screen.getByRole("button", { name: "Refresh rows" }));
		await user.pointer([
			{ target: checkboxFor("charlie") },
			{ keys: "[/MouseLeft]" },
		]);
		expect(onSelectionChange).not.toHaveBeenCalled();
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
		await user.click(checkboxFor("delta"));
		expect(onCheckboxClick).toHaveBeenCalled();
		expect(onSelectionChange).toHaveBeenLastCalledWith(
			new Set(["alpha", "charlie", "delta"]),
		);
	});
});

import type { Meta, StoryObj } from "@storybook/react-vite";
import type React from "react";
import { useState } from "react";
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

const mockInvoices = [
	{ id: "inv-101", name: "INV-101", status: "Paid", amount: "$250.00" },
	{ id: "inv-102", name: "INV-102", status: "Pending", amount: "$150.00" },
	{
		id: "inv-103",
		name: "INV-103",
		status: "Processing",
		amount: "$350.00",
		disabled: true,
	},
	{ id: "inv-104", name: "INV-104", status: "Paid", amount: "$450.00" },
];

type Invoice = (typeof mockInvoices)[number];

type ExampleTableProps = {
	rows?: readonly Invoice[];
	initialSelectedIds?: readonly string[];
};

const ExampleTable: React.FC<ExampleTableProps> = ({
	rows = mockInvoices,
	initialSelectedIds = [],
}) => {
	const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(
		new Set(initialSelectedIds),
	);
	return (
		<SelectableTable
			aria-label="Invoices"
			rows={rows}
			selectedIds={selectedIds}
			onSelectionChange={setSelectedIds}
		>
			<TableHeader>
				<TableRow>
					<TableHead>
						<SelectableTableSelectAll aria-label="Select all invoices" />
						Invoice
					</TableHead>
					<TableHead>Status</TableHead>
					<TableHead className="text-right">Amount</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{rows.length === 0 ? (
					<TableRow>
						<TableCell colSpan={3}>No invoices found.</TableCell>
					</TableRow>
				) : (
					rows.map((row) => (
						<SelectableTableRow key={row.id} rowId={row.id}>
							<TableCell>
								<SelectableTableCheckbox aria-label={`Select ${row.name}`} />
								{row.name}
							</TableCell>
							<TableCell>{row.status}</TableCell>
							<TableCell className="text-right">{row.amount}</TableCell>
						</SelectableTableRow>
					))
				)}
			</TableBody>
		</SelectableTable>
	);
};

const meta: Meta<typeof ExampleTable> = {
	title: "components/Table/SelectableTable",
	component: ExampleTable,
};
export default meta;
type Story = StoryObj<typeof ExampleTable>;

export const Default: Story = {};

export const Selected: Story = {
	args: { initialSelectedIds: ["inv-101", "inv-104"] },
};

export const Disabled: Story = {
	args: { rows: [mockInvoices[2]] },
};

export const Empty: Story = {
	args: { rows: [] },
};

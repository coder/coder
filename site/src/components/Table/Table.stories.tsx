import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./Table";

const invoices = [
	{
		invoice: "INV001",
		paymentStatus: "Paid",
		totalAmount: "$250.00",
		paymentMethod: "Credit Card",
	},
	{
		invoice: "INV002",
		paymentStatus: "Pending",
		totalAmount: "$150.00",
		paymentMethod: "PayPal",
	},
	{
		invoice: "INV003",
		paymentStatus: "Unpaid",
		totalAmount: "$350.00",
		paymentMethod: "Bank Transfer",
	},
	{
		invoice: "INV004",
		paymentStatus: "Paid",
		totalAmount: "$450.00",
		paymentMethod: "Credit Card",
	},
	{
		invoice: "INV005",
		paymentStatus: "Paid",
		totalAmount: "$550.00",
		paymentMethod: "PayPal",
	},
	{
		invoice: "INV006",
		paymentStatus: "Pending",
		totalAmount: "$200.00",
		paymentMethod: "Bank Transfer",
	},
	{
		invoice: "INV007",
		paymentStatus: "Unpaid",
		totalAmount: "$300.00",
		paymentMethod: "Credit Card",
	},
];

const meta: Meta<typeof Table> = {
	title: "components/Table",
	component: Table,
	args: {
		children: (
			<>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[100px]">Invoice</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Method</TableHead>
						<TableHead className="text-right">Amount</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{invoices.map((invoice) => (
						<TableRow key={invoice.invoice}>
							<TableCell className="font-medium">{invoice.invoice}</TableCell>
							<TableCell>{invoice.paymentStatus}</TableCell>
							<TableCell>{invoice.paymentMethod}</TableCell>
							<TableCell className="text-right">
								{invoice.totalAmount}
							</TableCell>
						</TableRow>
					))}
				</TableBody>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Table>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const invoiceHeader = canvas.getByRole("columnheader", { name: "Invoice" });

		expect(invoiceHeader).toHaveAttribute("scope", "col");
	},
};

export const ScopeOverride: Story = {
	args: {
		children: (
			<TableHeader>
				<TableRow>
					<TableHead scope="row">Invoice</TableHead>
				</TableRow>
			</TableHeader>
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const invoiceHeader = canvas.getByRole("rowheader", { name: "Invoice" });

		expect(invoiceHeader).toHaveAttribute("scope", "row");
	},
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

const meta: Meta<typeof PaginationWidgetBase> = {
	title: "components/PaginationWidgetBase",
	component: PaginationWidgetBase,
	args: {
		currentPage: 1,
		pageSize: 12,
		totalRecords: 200,
	},
};

export default meta;
type Story = StoryObj<typeof PaginationWidgetBase>;

export const MoreThan8Pages: Story = {};

export const LessThan8Pages: Story = {
	args: { totalRecords: 84 },
};

export const MoreThan7PagesWithActivePageCloseToStart: Story = {
	args: { currentPage: 2, pageSize: 12 },
};

export const MoreThan7PagesWithActivePageFarFromBoundaries: Story = {
	args: { currentPage: 4, pageSize: 12 },
};

export const MoreThan7PagesWithActivePageCloseToEnd: Story = {
	args: { currentPage: 17, pageSize: 12 },
};

/**
 * Demonstrates all pagination states from the design spec:
 * - 2 pages, first page active
 * - Last page active with left ellipsis
 * - Middle page active with both ellipses
 * - First page active with right ellipsis
 */
export const AllStates: Story = {
	render: () => (
		<div className="flex flex-col items-center gap-8 py-8">
			{/* 2 pages, page 1 active */}
			<PaginationWidgetBase
				currentPage={1}
				pageSize={25}
				totalRecords={50}
				onPageChange={() => {}}
			/>
			{/* Last page active, left ellipsis */}
			<PaginationWidgetBase
				currentPage={10}
				pageSize={25}
				totalRecords={250}
				onPageChange={() => {}}
			/>
			{/* Middle page active, both ellipses */}
			<PaginationWidgetBase
				currentPage={6}
				pageSize={25}
				totalRecords={250}
				onPageChange={() => {}}
			/>
			{/* First page active, right ellipsis */}
			<PaginationWidgetBase
				currentPage={1}
				pageSize={25}
				totalRecords={250}
				onPageChange={() => {}}
			/>
		</div>
	),
};

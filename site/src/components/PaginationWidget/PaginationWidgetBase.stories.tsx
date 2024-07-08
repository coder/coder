import type { Meta, StoryObj } from "@storybook/react";
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

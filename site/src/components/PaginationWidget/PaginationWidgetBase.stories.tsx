import { PaginationWidgetBase } from "./PaginationWidgetBase";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof PaginationWidgetBase> = {
  title: "components/PaginationWidgetBase",
  component: PaginationWidgetBase,
  args: {
    page: 1,
    limit: 12,
    count: 200,
  },
};

export default meta;
type Story = StoryObj<typeof PaginationWidgetBase>;

export const MoreThan8Pages: Story = {};

export const LessThan8Pages: Story = {
  args: {
    count: 84,
  },
};

export const MoreThan7PagesWithActivePageCloseToStart: Story = {
  args: { page: 2, limit: 12 },
};

export const MoreThan7PagesWithActivePageFarFromBoundaries: Story = {
  args: { page: 4, limit: 12 },
};

export const MoreThan7PagesWithActivePageCloseToEnd: Story = {
  args: { page: 17, limit: 12 },
};

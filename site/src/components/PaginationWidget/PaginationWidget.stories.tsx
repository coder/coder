import { PaginationWidget } from "./PaginationWidget";
import { createPaginationRef } from "./utils";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof PaginationWidget> = {
  title: "components/PaginationWidget",
  component: PaginationWidget,
  args: {
    prevLabel: "Previous",
    nextLabel: "Next",
    paginationRef: createPaginationRef({ page: 1, limit: 12 }),
    numRecords: 200,
  },
};

export default meta;
type Story = StoryObj<typeof PaginationWidget>;

export const MoreThan8Pages: Story = {};

export const LessThan8Pages: Story = {
  args: {
    numRecords: 84,
  },
};

export const MoreThan7PagesWithActivePageCloseToStart: Story = {
  args: {
    paginationRef: createPaginationRef({ page: 2, limit: 12 }),
  },
};

export const MoreThan7PagesWithActivePageFarFromBoundaries: Story = {
  args: {
    paginationRef: createPaginationRef({ page: 4, limit: 12 }),
  },
};

export const MoreThan7PagesWithActivePageCloseToEnd: Story = {
  args: {
    paginationRef: createPaginationRef({ page: 17, limit: 12 }),
  },
};

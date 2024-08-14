import type { Meta, StoryObj } from "@storybook/react";
import { PaginationStatus, TableToolbar } from "./TableToolbar";

const meta: Meta<typeof TableToolbar> = {
  title: "components/TableToolbar",
  component: TableToolbar,
};

export default meta;
type Story = StoryObj<typeof TableToolbar>;

export const WithPagination: Story = {
  args: {
    children: (
      <PaginationStatus
        isLoading={false}
        label="items"
        showing={10}
        total={100}
      />
    ),
  },
};

export const WithLoadingPagination: Story = {
  args: {
    children: <PaginationStatus isLoading />,
  },
};

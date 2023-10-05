import { Meta, StoryObj } from "@storybook/react";
import { MockBuilds } from "testHelpers/entities";
import { BuildsTable } from "./BuildsTable";

const meta: Meta<typeof BuildsTable> = {
  title: "components/BuildsTable",
  component: BuildsTable,
};

export default meta;
type Story = StoryObj<typeof BuildsTable>;

export const Example: Story = {
  args: {
    builds: MockBuilds,
    hasMoreBuilds: true,
  },
};

export const Empty: Story = {
  args: {
    builds: [],
  },
};

export const NoMoreBuilds: Story = {
  args: {
    builds: MockBuilds,
    hasMoreBuilds: false,
  },
};

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
  },
};

export const Empty: Story = {
  args: {
    builds: [],
  },
};

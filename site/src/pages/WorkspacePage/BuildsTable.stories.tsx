import { ComponentMeta, Story } from "@storybook/react";
import { MockBuilds } from "testHelpers/entities";
import { BuildsTable, BuildsTableProps } from "./BuildsTable";

export default {
  title: "components/BuildsTable",
  component: BuildsTable,
} as ComponentMeta<typeof BuildsTable>;

const Template: Story<BuildsTableProps> = (args) => <BuildsTable {...args} />;

export const Example = Template.bind({});
Example.args = {
  builds: MockBuilds,
};

export const Empty = Template.bind({});
Empty.args = {
  builds: [],
};

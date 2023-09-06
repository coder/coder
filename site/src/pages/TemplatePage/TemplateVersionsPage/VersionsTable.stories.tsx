import { action } from "@storybook/addon-actions";
import { ComponentMeta, Story } from "@storybook/react";
import { MockTemplateVersion } from "testHelpers/entities";
import { VersionsTable, VersionsTableProps } from "./VersionsTable";

export default {
  title: "components/VersionsTable",
  component: VersionsTable,
} as ComponentMeta<typeof VersionsTable>;

const Template: Story<VersionsTableProps> = (args) => (
  <VersionsTable {...args} />
);

export const Example = Template.bind({});
Example.args = {
  activeVersionId: MockTemplateVersion.id,
  versions: [
    {
      ...MockTemplateVersion,
      id: "2",
      name: "test-template-version-2",
      created_at: "2022-05-18T18:39:01.382927298Z",
    },
    MockTemplateVersion,
  ],
  onPromoteClick: undefined,
};

export const CanPromote = Template.bind({});
CanPromote.args = {
  activeVersionId: MockTemplateVersion.id,
  onPromoteClick: action("onPromoteClick"),
  versions: [
    {
      ...MockTemplateVersion,
      id: "2",
      name: "test-template-version-2",
      created_at: "2022-05-18T18:39:01.382927298Z",
    },
    MockTemplateVersion,
  ],
};

export const Empty = Template.bind({});
Empty.args = {
  versions: [],
};

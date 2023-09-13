import { action } from "@storybook/addon-actions";
import { MockTemplateVersion } from "testHelpers/entities";
import { VersionsTable } from "./VersionsTable";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof VersionsTable> = {
  title: "components/VersionsTable",
  component: VersionsTable,
};

export default meta;
type Story = StoryObj<typeof VersionsTable>;

export const Example: Story = {
  args: {
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
  },
};

export const CanPromote: Story = {
  args: {
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
  },
};

export const Empty: Story = {
  args: {
    versions: [],
  },
};

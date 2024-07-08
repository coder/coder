import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockCanceledProvisionerJob,
  MockCancelingProvisionerJob,
  MockFailedProvisionerJob,
  MockPendingProvisionerJob,
  MockRunningProvisionerJob,
  MockTemplateVersion,
} from "testHelpers/entities";
import { VersionsTable } from "./VersionsTable";

const meta: Meta<typeof VersionsTable> = {
  title: "pages/TemplatePage/VersionsTable",
  component: VersionsTable,
  args: {
    onPromoteClick: () => {},
    onArchiveClick: () => {},
  },
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
  },
};

export const NoEditPermission: Story = {
  args: {
    ...Example.args,
    onPromoteClick: undefined,
    onArchiveClick: undefined,
  },
};

export const BuildStatuses: Story = {
  args: {
    activeVersionId: MockTemplateVersion.id,
    onPromoteClick: action("onPromoteClick"),
    versions: [
      {
        ...MockTemplateVersion,
        id: "6",
        name: "test-version-6",
        created_at: "2022-05-18T18:39:01.382927298Z",
        job: MockCancelingProvisionerJob,
      },
      {
        ...MockTemplateVersion,
        id: "5",
        name: "test-version-5",
        created_at: "2022-05-18T18:39:01.382927298Z",
        job: MockCanceledProvisionerJob,
      },
      {
        ...MockTemplateVersion,
        id: "4",
        name: "test-version-4",
        created_at: "2022-05-18T18:39:01.382927298Z",
        job: MockRunningProvisionerJob,
      },
      {
        ...MockTemplateVersion,
        id: "3",
        name: "test-version-3",
        created_at: "2022-05-18T18:39:01.382927298Z",
        job: MockPendingProvisionerJob,
      },
      {
        ...MockTemplateVersion,
        id: "2",
        name: "test-version-2",
        created_at: "2022-05-18T18:39:01.382927298Z",
        job: MockFailedProvisionerJob,
      },
      MockTemplateVersion,
    ],
  },
};

export const BuildStatusesNoEditPermission: Story = {
  args: {
    ...BuildStatuses.args,
    onPromoteClick: undefined,
    onArchiveClick: undefined,
  },
};

export const Empty: Story = {
  args: {
    versions: [],
  },
};

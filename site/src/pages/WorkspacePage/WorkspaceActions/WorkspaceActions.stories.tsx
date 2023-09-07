import { action } from "@storybook/addon-actions";
import { Story } from "@storybook/react";
import * as Mocks from "../../../testHelpers/entities";
import { WorkspaceActions, WorkspaceActionsProps } from "./WorkspaceActions";

export default {
  title: "components/WorkspaceActions",
  component: WorkspaceActions,
};

const Template: Story<WorkspaceActionsProps> = (args) => (
  <WorkspaceActions {...args} />
);

const defaultArgs = {
  handleStart: action("start"),
  handleStop: action("stop"),
  handleRestart: action("restart"),
  handleDelete: action("delete"),
  handleUpdate: action("update"),
  handleCancel: action("cancel"),
  isOutdated: false,
  isUpdating: false,
};

export const Starting = Template.bind({});
Starting.args = {
  ...defaultArgs,
  workspace: Mocks.MockStartingWorkspace,
};

export const Running = Template.bind({});
Running.args = {
  ...defaultArgs,
  workspace: Mocks.MockWorkspace,
};

export const Stopping = Template.bind({});
Stopping.args = {
  ...defaultArgs,
  workspace: Mocks.MockStoppingWorkspace,
};

export const Stopped = Template.bind({});
Stopped.args = {
  ...defaultArgs,
  workspace: Mocks.MockStoppedWorkspace,
};

export const Canceling = Template.bind({});
Canceling.args = {
  ...defaultArgs,
  workspace: Mocks.MockCancelingWorkspace,
};

export const Canceled = Template.bind({});
Canceled.args = {
  ...defaultArgs,
  workspace: Mocks.MockCanceledWorkspace,
};

export const Deleting = Template.bind({});
Deleting.args = {
  ...defaultArgs,
  workspace: Mocks.MockDeletingWorkspace,
};

export const Deleted = Template.bind({});
Deleted.args = {
  ...defaultArgs,
  workspace: Mocks.MockDeletedWorkspace,
};

export const Outdated = Template.bind({});
Outdated.args = {
  ...defaultArgs,
  workspace: Mocks.MockOutdatedWorkspace,
};

export const Failed = Template.bind({});
Failed.args = {
  ...defaultArgs,
  workspace: Mocks.MockFailedWorkspace,
};

export const Updating = Template.bind({});
Updating.args = {
  ...defaultArgs,
  isUpdating: true,
  workspace: Mocks.MockOutdatedWorkspace,
};

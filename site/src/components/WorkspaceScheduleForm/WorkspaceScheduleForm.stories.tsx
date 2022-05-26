import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import React from "react"
import { WorkspaceScheduleForm, WorkspaceScheduleFormProps } from "./WorkspaceScheduleForm"

export default {
  title: "components/WorkspaceScheduleForm",
  component: WorkspaceScheduleForm,
}

const Template: Story<WorkspaceScheduleFormProps> = (args) => <WorkspaceScheduleForm {...args} />

export const Example = Template.bind({})
Example.args = {
  onCancel: () => action("onCancel"),
  onSubmit: () => action("onSubmit"),
}

import IconButton from "@mui/material/IconButton"
import EditIcon from "@mui/icons-material/Edit"
import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { WorkspaceSection, WorkspaceSectionProps } from "./WorkspaceSection"

export default {
  title: "components/WorkspaceSection",
  component: WorkspaceSection,
}

const Template: Story<WorkspaceSectionProps> = (args) => (
  <WorkspaceSection {...args}>Content</WorkspaceSection>
)

export const NoAction = Template.bind({})
NoAction.args = {
  title: "A Workspace Section",
}

export const Action = Template.bind({})
Action.args = {
  action: (
    <IconButton onClick={action("edit")} size="large">
      <EditIcon />
    </IconButton>
  ),
  title: "Action Section",
}

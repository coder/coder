import IconButton from "@material-ui/core/IconButton"
import EditIcon from "@material-ui/icons/Edit"
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
    <IconButton onClick={action("edit")}>
      <EditIcon />
    </IconButton>
  ),
  title: "Action Section",
}

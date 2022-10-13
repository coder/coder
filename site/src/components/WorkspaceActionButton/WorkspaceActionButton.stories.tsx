import PlayArrowRoundedIcon from "@material-ui/icons/PlayArrowRounded"
import { ComponentMeta, Story } from "@storybook/react"
import {
  WorkspaceActionButton,
  WorkspaceActionButtonProps,
} from "./WorkspaceActionButton"

export default {
  title: "components/WorkspaceActionButton",
  component: WorkspaceActionButton,
} as ComponentMeta<typeof WorkspaceActionButton>

const Template: Story<WorkspaceActionButtonProps> = (args) => (
  <WorkspaceActionButton {...args} />
)

export const Example = Template.bind({})
Example.args = {
  icon: <PlayArrowRoundedIcon />,
  label: "Start workspace",
}

import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import {
  WorkspaceDeletedBanner,
  WorkspaceDeletedBannerProps,
} from "./WorkspaceDeletedBanner"

export default {
  title: "components/WorkspaceDeletedBanner",
  component: WorkspaceDeletedBanner,
}

const Template: Story<WorkspaceDeletedBannerProps> = (args) => (
  <WorkspaceDeletedBanner {...args} />
)

export const Example = Template.bind({})
Example.args = {
  handleClick: action("extend"),
}

import { Story } from "@storybook/react"
import {
  WorkspaceAppErrorPageView,
  WorkspaceAppErrorPageViewProps,
} from "./WorkspaceAppErrorPageView"

export default {
  title: "pages/WorkspaceAppErrorPageView",
  component: WorkspaceAppErrorPageView,
}

const Template: Story<WorkspaceAppErrorPageViewProps> = (args) => (
  <WorkspaceAppErrorPageView {...args} />
)

export const NotRunning = Template.bind({})
NotRunning.args = {
  appName: "code-server",
  // This is a real message copied and pasted from the backend!
  message:
    "remote dial error: dial 'tcp://localhost:13337': dial tcp 127.0.0.1:13337: connect: connection refused",
}

import { ComponentMeta, Story } from "@storybook/react"
import { MockWorkspaceBuild, MockWorkspaceBuildLogs } from "../../testHelpers/entities"
import { WorkspaceBuildPageView, WorkspaceBuildPageViewProps } from "./WorkspaceBuildPageView"

export default {
  title: "pages/WorkspaceBuildPageView",
  component: WorkspaceBuildPageView,
} as ComponentMeta<typeof WorkspaceBuildPageView>

const Template: Story<WorkspaceBuildPageViewProps> = (args) => <WorkspaceBuildPageView {...args} />

export const Example = Template.bind({})
Example.args = {
  build: MockWorkspaceBuild,
  logs: MockWorkspaceBuildLogs,
  isWaitingForLogs: false,
}

import { Story } from "@storybook/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceScheduleBanner, WorkspaceScheduleBannerProps } from "./WorkspaceScheduleBanner"

dayjs.extend(utc)

export default {
  title: "components/WorkspaceScheduleBanner",
  component: WorkspaceScheduleBanner,
}

const Template: Story<WorkspaceScheduleBannerProps> = (args) => <WorkspaceScheduleBanner {...args} />

export const Example = Template.bind({})
Example.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: dayjs().utc().format(),
      job: {
        ...Mocks.MockProvisionerJob,
        status: "succeeded",
      },
      transition: "start",
    },
    ttl_ms: 2 * 60 * 60 * 1000, // 2 hours
  },
}

import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/entities"
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
  workspace: {
    ...Mocks.MockWorkspace,

    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      job: {
        ...Mocks.MockProvisionerJob,
        status: "succeeded",
      },
      transition: "delete",
    },
  },
}

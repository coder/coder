import { ComponentMeta, Story } from "@storybook/react"
import dayjs from "dayjs"
import {
  MockProvisionerJob,
  MockStartingWorkspace,
  MockWorkspaceBuild,
} from "../../testHelpers/renderHelpers"
import {
  WorkspaceBuildProgress,
  WorkspaceBuildProgressProps,
} from "./WorkspaceBuildProgress"

export default {
  title: "components/WorkspaceBuildProgress",
  component: WorkspaceBuildProgress,
} as ComponentMeta<typeof WorkspaceBuildProgress>

const Template: Story<WorkspaceBuildProgressProps> = (args) => (
  <WorkspaceBuildProgress {...args} />
)

export const Starting = Template.bind({})
Starting.args = {
  buildEstimate: 10000,
  workspace: {
    ...MockStartingWorkspace,
    latest_build: {
      ...MockWorkspaceBuild,
      status: "starting",
      job: {
        ...MockProvisionerJob,
        started_at: dayjs().add(-5, "second").format(),
        status: "running",
      },
    },
  },
}

export const StartingUnknown = Template.bind({})
StartingUnknown.args = {
  ...Starting.args,
  buildEstimate: undefined,
}

export const StartingPassedEstimate = Template.bind({})
StartingPassedEstimate.args = {
  ...Starting.args,
  buildEstimate: 1000,
}

import { ComponentMeta, Story } from "@storybook/react"
import {
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockTemplateVersionParameter4,
  MockWorkspace,
} from "testHelpers/entities"
import {
  WorkspaceBuildParametersPageView,
  WorkspaceBuildParametersPageViewProps,
} from "./WorkspaceBuildParametersPageView"

export default {
  title: "pages/WorkspaceBuildParametersPageView",
  component: WorkspaceBuildParametersPageView,
} as ComponentMeta<typeof WorkspaceBuildParametersPageView>

const Template: Story<WorkspaceBuildParametersPageViewProps> = (args) => (
  <WorkspaceBuildParametersPageView {...args} />
)

export const NoRichParametersDefined = Template.bind({})
NoRichParametersDefined.args = {
  workspace: MockWorkspace,
  templateParameters: [],
  workspaceBuildParameters: [],
  updateWorkspaceErrors: {},
  initialTouched: {
    name: true,
  },
}

export const RichParametersDefined = Template.bind({})
RichParametersDefined.args = {
  workspace: MockWorkspace,
  templateParameters: [
    MockTemplateVersionParameter1,
    MockTemplateVersionParameter2,
    MockTemplateVersionParameter3,
    MockTemplateVersionParameter4,
  ],
  workspaceBuildParameters: [],
  updateWorkspaceErrors: {},
  initialTouched: {
    name: true,
  },
}

import { ComponentMeta, Story } from "@storybook/react"
import {
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockWorkspace,
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
} from "testHelpers/entities"
import {
  WorkspaceSettingsPageView,
  WorkspaceSettingsPageViewProps,
} from "./WorkspaceSettingsPageView"

export default {
  title: "pages/WorkspaceSettingsPageView",
  component: WorkspaceSettingsPageView,
  args: {
    formError: undefined,
    loadingError: undefined,
    isLoading: false,
    isSubmitting: false,
    settings: {
      workspace: MockWorkspace,
      buildParameters: [
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
      ],
      templateVersionRichParameters: [
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
      ],
    },
  },
} as ComponentMeta<typeof WorkspaceSettingsPageView>

const Template: Story<WorkspaceSettingsPageViewProps> = (args) => (
  <WorkspaceSettingsPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {}

import { action } from "@storybook/addon-actions"
import { ComponentMeta, Story } from "@storybook/react"
import {
  makeMockApiError,
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersion2,
  MockUser,
  MockWorkspace,
} from "testHelpers/entities"
import {
  WorkspaceChangeVersionPageView,
  WorkspaceChangeVersionPageViewProps,
} from "./WorkspaceChangeVersionPageView"

export default {
  title: "pages/WorkspaceChangeVersionPageView",
  component: WorkspaceChangeVersionPageView,
} as ComponentMeta<typeof WorkspaceChangeVersionPageView>

const Template: Story<WorkspaceChangeVersionPageViewProps> = (args) => (
  <WorkspaceChangeVersionPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {
  isUpdating: false,
  onSubmit: action("submit"),
  context: {
    error: undefined,
    owner: MockUser.username,
    workspaceName: MockWorkspace.name,
    template: MockTemplate,
    templateVersions: [MockTemplateVersion2, MockTemplateVersion],
    workspace: MockWorkspace,
  },
}

export const Error = Template.bind({})
Error.args = {
  isUpdating: false,
  onSubmit: action("submit"),
  context: {
    error: makeMockApiError({ message: "Error on updating the version." }),
    owner: MockUser.username,
    workspaceName: MockWorkspace.name,
    template: MockTemplate,
    templateVersions: [MockTemplateVersion2, MockTemplateVersion],
    workspace: MockWorkspace,
  },
}

import { Story } from "@storybook/react"
import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionFileTree,
  MockWorkspaceBuildLogs,
  MockWorkspaceResource,
  MockWorkspaceResource2,
  MockWorkspaceResource3,
} from "testHelpers/entities"
import {
  TemplateVersionEditor,
  TemplateVersionEditorProps,
} from "./TemplateVersionEditor"

export default {
  title: "components/TemplateVersionEditor",
  component: TemplateVersionEditor,
  parameters: {
    layout: "fullscreen",
  },
}

const Template: Story<TemplateVersionEditorProps> = (
  args: TemplateVersionEditorProps,
) => <TemplateVersionEditor {...args} />

export const Example = Template.bind({})
Example.args = {
  defaultFileTree: MockTemplateVersionFileTree,
  template: MockTemplate,
  templateVersion: MockTemplateVersion,
}

export const Logs = Template.bind({})
Logs.args = {
  ...Example.args,
  buildLogs: MockWorkspaceBuildLogs,
}

export const Resources = Template.bind({})
Resources.args = {
  ...Example.args,
  buildLogs: MockWorkspaceBuildLogs,
  resources: [
    MockWorkspaceResource,
    MockWorkspaceResource2,
    MockWorkspaceResource3,
  ],
}

export const SuccessfulPublish = Template.bind({})
SuccessfulPublish.args = {
  ...Example.args,
  publishedVersion: MockTemplateVersion,
}

import { ComponentMeta, Story } from "@storybook/react"
import {
  mockApiError,
  MockTemplate,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
} from "../../testHelpers/entities"
import {
  CreateWorkspaceErrors,
  CreateWorkspacePageView,
  CreateWorkspacePageViewProps,
} from "./CreateWorkspacePageView"

export default {
  title: "pages/CreateWorkspacePageView",
  component: CreateWorkspacePageView,
} as ComponentMeta<typeof CreateWorkspacePageView>

const Template: Story<CreateWorkspacePageViewProps> = (args) => (
  <CreateWorkspacePageView {...args} />
)

export const NoParameters = Template.bind({})
NoParameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  createWorkspaceErrors: {},
}

export const Parameters = Template.bind({})
Parameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  createWorkspaceErrors: {},
}

export const RedisplayParameters = Template.bind({})
RedisplayParameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  createWorkspaceErrors: {},
}

export const GetTemplatesError = Template.bind({})
GetTemplatesError.args = {
  ...Parameters.args,
  createWorkspaceErrors: {
    [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: mockApiError({
      message: "Failed to fetch templates.",
      detail: "You do not have permission to access this resource.",
    }),
  },
  hasTemplateErrors: true,
}

export const CreateWorkspaceError = Template.bind({})
CreateWorkspaceError.args = {
  ...Parameters.args,
  createWorkspaceErrors: {
    [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: mockApiError({
      message:
        'Workspace "test" already exists in the "docker-amd64" template.',
      validations: [
        {
          field: "name",
          detail: "This value is already in use and should be unique.",
        },
      ],
    }),
  },
  initialTouched: {
    name: true,
  },
}

export const RichParameters = Template.bind({})
RichParameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  templateParameters: [
    MockTemplateVersionParameter1,
    MockTemplateVersionParameter2,
    MockTemplateVersionParameter3,
    {
      name: "Region",
      required: false,
      description: "",
      description_plaintext: "",
      type: "string",
      mutable: false,
      default_value: "",
      icon: "/emojis/1f30e.png",
      options: [
        {
          name: "Pittsburgh",
          description: "",
          value: "us-pittsburgh",
          icon: "/emojis/1f1fa-1f1f8.png",
        },
        {
          name: "Helsinki",
          description: "",
          value: "eu-helsinki",
          icon: "/emojis/1f1eb-1f1ee.png",
        },
        {
          name: "Sydney",
          description: "",
          value: "ap-sydney",
          icon: "/emojis/1f1e6-1f1fa.png",
        },
      ],
      ephemeral: false,
    },
  ],
  createWorkspaceErrors: {},
}

export const GitAuth = Template.bind({})
GitAuth.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  createWorkspaceErrors: {},
  templateParameters: [],
  templateGitAuth: [
    {
      id: "github",
      type: "github",
      authenticated: false,
      authenticate_url: "",
    },
    {
      id: "gitlab",
      type: "gitlab",
      authenticated: true,
      authenticate_url: "",
    },
  ],
}

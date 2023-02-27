import { ComponentMeta, Story } from "@storybook/react"
import {
  makeMockApiError,
  mockParameterSchema,
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
  templateSchema: [],
  createWorkspaceErrors: {},
}

export const Parameters = Template.bind({})
Parameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  templateSchema: [
    mockParameterSchema({
      name: "region",
      default_source_value: "🏈 US Central",
      description: "Where would you like your workspace to live?",
      redisplay_value: true,
      validation_contains: [
        "🏈 US Central",
        "⚽ Brazil East",
        "💶 EU West",
        "🦘 Australia South",
      ],
    }),
    mockParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should you instance be?",
      validation_contains: ["Small", "Medium", "Big"],
      redisplay_value: true,
    }),
    mockParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should your instance be?",
      validation_contains: ["Small", "Medium", "Big"],
      redisplay_value: true,
    }),
    mockParameterSchema({
      name: "disable_docker",
      description: "Disable Docker?",
      validation_value_type: "bool",
      default_source_value: "false",
      redisplay_value: true,
    }),
  ],
  createWorkspaceErrors: {},
}

export const RedisplayParameters = Template.bind({})
RedisplayParameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  templateSchema: [
    mockParameterSchema({
      name: "region",
      default_source_value: "🏈 US Central",
      description: "Where would you like your workspace to live?",
      redisplay_value: false,
      validation_contains: [
        "🏈 US Central",
        "⚽ Brazil East",
        "💶 EU West",
        "🦘 Australia South",
      ],
    }),
    mockParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should you instance be?",
      validation_contains: ["Small", "Medium", "Big"],
      redisplay_value: false,
    }),
    mockParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should your instance be?",
      validation_contains: ["Small", "Medium", "Big"],
      redisplay_value: true,
    }),
    mockParameterSchema({
      name: "disable_docker",
      description: "Disable Docker?",
      validation_value_type: "bool",
      default_source_value: "false",
      redisplay_value: true,
    }),
  ],
  createWorkspaceErrors: {},
}

export const GetTemplatesError = Template.bind({})
GetTemplatesError.args = {
  ...Parameters.args,
  createWorkspaceErrors: {
    [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: makeMockApiError({
      message: "Failed to fetch templates.",
      detail: "You do not have permission to access this resource.",
    }),
  },
  hasTemplateErrors: true,
}

export const GetTemplateSchemaError = Template.bind({})
GetTemplateSchemaError.args = {
  ...Parameters.args,
  createWorkspaceErrors: {
    [CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR]: makeMockApiError({
      message: 'Failed to fetch template schema for "docker-amd64".',
      detail: "You do not have permission to access this resource.",
    }),
  },
  hasTemplateErrors: true,
}

export const CreateWorkspaceError = Template.bind({})
CreateWorkspaceError.args = {
  ...Parameters.args,
  createWorkspaceErrors: {
    [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: makeMockApiError({
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

import { ComponentMeta, Story } from "@storybook/react"
import { ParameterSchema } from "../../api/typesGenerated"
import { makeMockApiError, MockTemplate } from "../../testHelpers/entities"
import {
  CreateWorkspaceErrors,
  CreateWorkspacePageView,
  CreateWorkspacePageViewProps,
} from "./CreateWorkspacePageView"

const createParameterSchema = (
  partial: Partial<ParameterSchema>,
): ParameterSchema => {
  return {
    id: "000000",
    job_id: "000000",
    allow_override_destination: false,
    allow_override_source: true,
    created_at: "",
    default_destination_scheme: "none",
    default_refresh: "",
    default_source_scheme: "data",
    default_source_value: "default-value",
    name: "parameter name",
    description: "Some description!",
    redisplay_value: false,
    validation_condition: "",
    validation_contains: [],
    validation_error: "",
    validation_type_system: "",
    validation_value_type: "",
    ...partial,
  }
}

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
    createParameterSchema({
      name: "region",
      default_source_value: "🏈 US Central",
      description: "Where would you like your workspace to live?",
      validation_contains: [
        "🏈 US Central",
        "⚽ Brazil East",
        "💶 EU West",
        "🦘 Australia South",
      ],
    }),
    createParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should you instance be?",
      validation_contains: ["Small", "Medium", "Big"],
    }),
    createParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should your instance be?",
      validation_contains: ["Small", "Medium", "Big"],
    }),
    createParameterSchema({
      name: "disable_docker",
      description: "Disable Docker?",
      validation_value_type: "bool",
      default_source_value: "false",
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

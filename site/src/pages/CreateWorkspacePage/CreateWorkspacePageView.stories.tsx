import { Meta, StoryObj } from "@storybook/react";
import {
  mockApiError,
  MockTemplate,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockUser,
} from "../../testHelpers/entities";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";

const meta: Meta<typeof CreateWorkspacePageView> = {
  title: "pages/CreateWorkspacePageView",
  component: CreateWorkspacePageView,
  args: {
    defaultName: "",
    defaultOwner: MockUser,
    defaultBuildParameters: [],
    template: MockTemplate,
    parameters: [],
    gitAuth: [],
    permissions: {
      createWorkspaceForUser: true,
    },
  },
};

export default meta;
type Story = StoryObj<typeof CreateWorkspacePageView>;

export const NoParameters: Story = {};

export const CreateWorkspaceError: Story = {
  args: {
    error: mockApiError({
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
};

export const Parameters: Story = {
  args: {
    parameters: [
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
  },
};

export const GitAuth: Story = {
  args: {
    gitAuth: [
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
  },
};

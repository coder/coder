import {
  mockApiError,
  MockOrganization,
  MockPermissions,
  MockTemplate,
  MockTemplateExample,
  MockTemplateExample2,
} from "../../testHelpers/entities";
import { TemplatesPageView } from "./TemplatesPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TemplatesPageView> = {
  title: "pages/TemplatesPageView",
  component: TemplatesPageView,
};

export default meta;
type Story = StoryObj<typeof TemplatesPageView>;

export const WithTemplates: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      permissions: MockPermissions,
      error: undefined,
      templates: [
        MockTemplate,
        {
          ...MockTemplate,
          active_user_count: -1,
          description: "🚀 Some new template that has no activity data",
          icon: "/icon/goland.svg",
        },
        {
          ...MockTemplate,
          active_user_count: 150,
          description: "😮 Wow, this one has a bunch of usage!",
          icon: "",
        },
        {
          ...MockTemplate,
          description:
            "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. ",
        },
      ],
      examples: [],
    },
  },
};

export const WithTemplatesSmallViewPort: Story = {
  args: {
    ...WithTemplates.args,
  },
  parameters: {
    chromatic: { viewports: [600] },
  },
};

export const EmptyCanCreate: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      permissions: MockPermissions,
      error: undefined,
      templates: [],
      examples: [MockTemplateExample, MockTemplateExample2],
    },
  },
};

export const EmptyCannotCreate: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      permissions: {
        ...MockPermissions,
        createTemplates: false,
      },
      error: undefined,
      templates: [],
      examples: [MockTemplateExample, MockTemplateExample2],
    },
  },
};

export const Error: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      permissions: {
        ...MockPermissions,
        createTemplates: false,
      },
      error: mockApiError({
        message: "Something went wrong fetching templates.",
      }),
      templates: undefined,
      examples: undefined,
    },
  },
};

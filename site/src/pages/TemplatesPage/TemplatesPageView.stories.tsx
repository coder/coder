import { ComponentMeta, Story } from "@storybook/react";
import {
  mockApiError,
  MockOrganization,
  MockPermissions,
  MockTemplate,
  MockTemplateExample,
  MockTemplateExample2,
} from "../../testHelpers/entities";
import { TemplatesPageView, TemplatesPageViewProps } from "./TemplatesPageView";

export default {
  title: "pages/TemplatesPageView",
  component: TemplatesPageView,
} as ComponentMeta<typeof TemplatesPageView>;

const Template: Story<TemplatesPageViewProps> = (args) => (
  <TemplatesPageView {...args} />
);

export const WithTemplates = Template.bind({});
WithTemplates.args = {
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
};

export const WithTemplatesSmallViewPort = Template.bind({});
WithTemplatesSmallViewPort.args = {
  ...WithTemplates.args,
};
WithTemplatesSmallViewPort.parameters = {
  chromatic: { viewports: [600] },
};

export const EmptyCanCreate = Template.bind({});
EmptyCanCreate.args = {
  context: {
    organizationId: MockOrganization.id,
    permissions: MockPermissions,
    error: undefined,
    templates: [],
    examples: [MockTemplateExample, MockTemplateExample2],
  },
};

export const EmptyCannotCreate = Template.bind({});
EmptyCannotCreate.args = {
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
};

export const Error = Template.bind({});
Error.args = {
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
};

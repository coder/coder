import { Story } from "@storybook/react";
import {
  mockApiError,
  MockOrganization,
  MockTemplateExample,
} from "testHelpers/entities";
import {
  StarterTemplatePageView,
  StarterTemplatePageViewProps,
} from "./StarterTemplatePageView";

export default {
  title: "pages/StarterTemplatePageView",
  component: StarterTemplatePageView,
};

const Template: Story<StarterTemplatePageViewProps> = (args) => (
  <StarterTemplatePageView {...args} />
);

export const Default = Template.bind({});
Default.args = {
  context: {
    exampleId: MockTemplateExample.id,
    organizationId: MockOrganization.id,
    error: undefined,
    starterTemplate: MockTemplateExample,
  },
};

export const Error = Template.bind({});
Error.args = {
  context: {
    exampleId: MockTemplateExample.id,
    organizationId: MockOrganization.id,
    error: mockApiError({
      message: `Example ${MockTemplateExample.id} not found.`,
    }),
    starterTemplate: undefined,
  },
};

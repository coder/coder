import { action } from "@storybook/addon-actions";
import { Story } from "@storybook/react";
import { UseTabResult } from "hooks/useTab";
import {
  mockApiError,
  MockOrganization,
  MockTemplate,
  MockTemplateVersion,
} from "testHelpers/entities";
import {
  TemplateVersionPageView,
  TemplateVersionPageViewProps,
} from "./TemplateVersionPageView";

export default {
  title: "pages/TemplateVersionPageView",
  component: TemplateVersionPageView,
};

const Template: Story<TemplateVersionPageViewProps> = (args) => (
  <TemplateVersionPageView {...args} />
);

const tab: UseTabResult = {
  value: "0",
  set: action("changeTab"),
};

const readmeContent = `---
name:Template test
---
## Instructions
You can add instructions here

[Some link info](https://coder.com)
\`\`\`
# This is a really long sentence to test that the code block wraps into a new line properly.
\`\`\``;

const defaultArgs: TemplateVersionPageViewProps = {
  tab,
  templateName: MockTemplate.name,
  versionName: MockTemplateVersion.name,
  context: {
    templateName: MockTemplate.name,
    orgId: MockOrganization.id,
    versionName: MockTemplateVersion.name,
    currentVersion: MockTemplateVersion,
    currentFiles: {
      "README.md": readmeContent,
      "main.tf": `{}`,
    },
  },
};

export const Default = Template.bind({});
Default.args = defaultArgs;

export const Error = Template.bind({});
Error.args = {
  ...defaultArgs,
  context: {
    ...defaultArgs.context,
    currentVersion: undefined,
    currentFiles: undefined,
    error: mockApiError({
      message: "Error on loading the template version",
    }),
  },
};

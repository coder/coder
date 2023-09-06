import { action } from "@storybook/addon-actions";
import { Story } from "@storybook/react";
import {
  mockApiError,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplateVersionVariable4,
  MockTemplateVersionVariable5,
} from "testHelpers/entities";
import {
  TemplateVariablesPageView,
  TemplateVariablesPageViewProps,
} from "./TemplateVariablesPageView";

export default {
  title: "pages/TemplateVariablesPageView",
  component: TemplateVariablesPageView,
};

const TemplateVariables: Story<TemplateVariablesPageViewProps> = (args) => (
  <TemplateVariablesPageView {...args} />
);

export const Loading = TemplateVariables.bind({});
Loading.args = {
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
};

export const Basic = TemplateVariables.bind({});
Basic.args = {
  templateVersion: MockTemplateVersion,
  templateVariables: [
    MockTemplateVersionVariable1,
    MockTemplateVersionVariable2,
    MockTemplateVersionVariable3,
    MockTemplateVersionVariable4,
  ],
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
};

// This example isn't fully supported. As "user_variable_values" is an array,
// FormikTouched can't properly handle this.
// See: https://github.com/jaredpalmer/formik/issues/2022
export const RequiredVariable = TemplateVariables.bind({});
RequiredVariable.args = {
  templateVersion: MockTemplateVersion,
  templateVariables: [
    MockTemplateVersionVariable4,
    MockTemplateVersionVariable5,
  ],
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
  initialTouched: {
    user_variable_values: true,
  },
};

export const WithUpdateTemplateError = TemplateVariables.bind({});
WithUpdateTemplateError.args = {
  templateVersion: MockTemplateVersion,
  templateVariables: [
    MockTemplateVersionVariable1,
    MockTemplateVersionVariable2,
    MockTemplateVersionVariable3,
    MockTemplateVersionVariable4,
  ],
  errors: {
    updateTemplateError: mockApiError({
      message: "Something went wrong.",
    }),
  },
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
};

export const WithJobError = TemplateVariables.bind({});
WithJobError.args = {
  templateVersion: MockTemplateVersion,
  templateVariables: [
    MockTemplateVersionVariable1,
    MockTemplateVersionVariable2,
    MockTemplateVersionVariable3,
    MockTemplateVersionVariable4,
  ],
  errors: {
    jobError:
      "template import provision for start: recv import provision: plan terraform: terraform plan: exit status 1",
  },
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
};

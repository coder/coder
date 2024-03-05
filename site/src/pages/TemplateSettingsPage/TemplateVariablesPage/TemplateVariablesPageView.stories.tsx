import type { Meta, StoryObj } from "@storybook/react";
import {
  mockApiError,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplateVersionVariable4,
  MockTemplateVersionVariable5,
} from "testHelpers/entities";
import { TemplateVariablesPageView } from "./TemplateVariablesPageView";

const meta: Meta<typeof TemplateVariablesPageView> = {
  title: "pages/TemplateSettingsPage/TemplateVariablesPageView",
  component: TemplateVariablesPageView,
};

export default meta;
type Story = StoryObj<typeof TemplateVariablesPageView>;

export const Loading: Story = {};

export const Basic: Story = {
  args: {
    templateVersion: MockTemplateVersion,
    templateVariables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
      MockTemplateVersionVariable4,
    ],
  },
};

// This example isn't fully supported. As "user_variable_values" is an array,
// FormikTouched can't properly handle this.
// See: https://github.com/jaredpalmer/formik/issues/2022
export const RequiredVariable: Story = {
  args: {
    templateVersion: MockTemplateVersion,
    templateVariables: [
      MockTemplateVersionVariable4,
      MockTemplateVersionVariable5,
    ],

    initialTouched: {
      user_variable_values: true,
    },
  },
};

export const WithErrors: Story = {
  args: {
    templateVersion: MockTemplateVersion,
    templateVariables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
      MockTemplateVersionVariable4,
    ],
    errors: {
      buildError: mockApiError({
        message: "buildError",
        validations: [
          {
            field: `user_variable_values[0].value`,
            detail: "Variable is required.",
          },
        ],
      }),
      publishError: mockApiError({ message: "publishError" }),
    },

    initialTouched: {
      user_variable_values: true,
    },
  },
};

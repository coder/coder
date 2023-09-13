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
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TemplateVariablesPageView> = {
  title: "pages/TemplateVariablesPageView",
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

export const WithUpdateTemplateError: Story = {
  args: {
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
  },
};

export const WithJobError: Story = {
  args: {
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
  },
};

import { action } from "@storybook/addon-actions";
import { Story } from "@storybook/react";
import { SetupPageView, SetupPageViewProps } from "./SetupPageView";
import { mockApiError } from "testHelpers/entities";

export default {
  title: "pages/SetupPageView",
  component: SetupPageView,
};

const Template: Story<SetupPageViewProps> = (args: SetupPageViewProps) => (
  <SetupPageView {...args} />
);

export const Ready = Template.bind({});
Ready.args = {
  onSubmit: action("submit"),
};

export const FormError = Template.bind({});
FormError.args = {
  onSubmit: action("submit"),
  error: mockApiError({
    validations: [{ field: "username", detail: "Username taken" }],
  }),
};

export const Loading = Template.bind({});
Loading.args = {
  onSubmit: action("submit"),
  isLoading: true,
};

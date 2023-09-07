import { Story } from "@storybook/react";
import {
  EnterpriseSnackbar,
  EnterpriseSnackbarProps,
} from "./EnterpriseSnackbar";

export default {
  title: "components/EnterpriseSnackbar",
  component: EnterpriseSnackbar,
};

const Template: Story<EnterpriseSnackbarProps> = (
  args: EnterpriseSnackbarProps,
) => <EnterpriseSnackbar {...args} />;

export const Error = Template.bind({});
Error.args = {
  variant: "error",
  open: true,
  message: "Oops, something wrong happened.",
};

export const Info = Template.bind({});
Info.args = {
  variant: "info",
  open: true,
  message: "Hey, something happened.",
};

export const Success = Template.bind({});
Success.args = {
  variant: "success",
  open: true,
  message: "Hey, something good happened.",
};

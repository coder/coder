import { Story } from "@storybook/react";
import { Pill, PillProps } from "./Pill";

export default {
  title: "components/Pill",
  component: Pill,
};

const Template: Story<PillProps> = (args) => <Pill {...args} />;

export const Primary = Template.bind({});
Primary.args = {
  text: "Primary",
  type: "primary",
};

export const Secondary = Template.bind({});
Secondary.args = {
  text: "Secondary",
  type: "secondary",
};

export const Success = Template.bind({});
Success.args = {
  text: "Success",
  type: "success",
};

export const Info = Template.bind({});
Info.args = {
  text: "Information",
  type: "info",
};

export const Warning = Template.bind({});
Warning.args = {
  text: "Warning",
  type: "warning",
};

export const Error = Template.bind({});
Error.args = {
  text: "Error",
  type: "error",
};

export const Default = Template.bind({});
Default.args = {
  text: "Default",
};

export const WarningLight = Template.bind({});
WarningLight.args = {
  text: "Warning",
  type: "warning",
  lightBorder: true,
};

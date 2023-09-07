import { Story } from "@storybook/react";
import { LoadingButton, LoadingButtonProps } from "./LoadingButton";

export default {
  title: "components/LoadingButton",
  component: LoadingButton,
  argTypes: {
    loading: { control: "boolean" },
    children: { control: "text" },
  },
  args: {
    children: "Create workspace",
  },
};

const Template: Story<LoadingButtonProps> = (args) => (
  <LoadingButton {...args} />
);

export const Loading = Template.bind({});
Loading.args = {
  loading: true,
};

export const NotLoading = Template.bind({});
NotLoading.args = {
  loading: false,
};

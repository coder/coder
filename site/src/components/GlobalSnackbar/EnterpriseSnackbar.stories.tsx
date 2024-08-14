import type { Meta, StoryObj } from "@storybook/react";
import { EnterpriseSnackbar } from "./EnterpriseSnackbar";

const meta: Meta<typeof EnterpriseSnackbar> = {
  title: "components/EnterpriseSnackbar",
  component: EnterpriseSnackbar,
};

export default meta;
type Story = StoryObj<typeof EnterpriseSnackbar>;

export const Error: Story = {
  args: {
    variant: "error",
    open: true,
    message: "Oops, something wrong happened.",
  },
};

export const Info: Story = {
  args: {
    variant: "info",
    open: true,
    message: "Hey, something happened.",
  },
};

export const Success: Story = {
  args: {
    variant: "success",
    open: true,
    message: "Hey, something good happened.",
  },
};

import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { FormFooter } from "./FormFooter";

const meta: Meta<typeof FormFooter> = {
  title: "components/FormFooter",
  component: FormFooter,
  args: {
    isLoading: false,
    onCancel: action("onCancel"),
  },
};

export default meta;
type Story = StoryObj<typeof FormFooter>;

export const Ready: Story = {
  args: {},
};

export const NoCancel: Story = {
  args: {
    onCancel: undefined,
  },
};

export const Custom: Story = {
  args: {
    submitLabel: "Create",
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};

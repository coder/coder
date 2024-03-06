import type { Meta, StoryObj } from "@storybook/react";
import { FormFooter } from "./FormFooter";

const meta: Meta<typeof FormFooter> = {
  title: "components/FormFooter",
  component: FormFooter,
};

export default meta;
type Story = StoryObj<typeof FormFooter>;

export const Ready: Story = {
  args: {
    isLoading: false,
  },
};

export const Custom: Story = {
  args: {
    isLoading: false,
    submitLabel: "Create",
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};

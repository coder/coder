import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, screen } from "@storybook/test";
import { CreateTemplateButton } from "./CreateTemplateButton";

const meta: Meta<typeof CreateTemplateButton> = {
  title: "pages/TemplatesPage/CreateTemplateButton",
  component: CreateTemplateButton,
};

export default meta;
type Story = StoryObj<typeof CreateTemplateButton>;

export const Close: Story = {};

export const Open: Story = {
  play: async ({ step }) => {
    const user = userEvent.setup();
    await step("click on trigger", async () => {
      await user.click(screen.getByRole("button"));
    });
  },
};

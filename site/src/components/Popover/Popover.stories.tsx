import Button from "@mui/material/Button";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, screen, userEvent, within, waitFor } from "@storybook/test";
import { Popover, PopoverTrigger, PopoverContent } from "./Popover";

const meta: Meta<typeof Popover> = {
  title: "components/Popover",
  component: Popover,
};

export default meta;
type Story = StoryObj<typeof Popover>;

const content = `
According to all known laws of aviation, there is no way a bee should be able to fly.
Its wings are too small to get its fat little body off the ground. The bee, of course,
flies anyway because bees don't care what humans think is impossible.
`;

export const Example: Story = {
  args: {
    children: (
      <>
        <PopoverTrigger>
          <Button>Click here!</Button>
        </PopoverTrigger>
        <PopoverContent>{content}</PopoverContent>
      </>
    ),
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("click to open", async () => {
      await userEvent.click(canvas.getByRole("button"));
      await waitFor(() =>
        expect(
          screen.getByText(/according to all known laws/i),
        ).toBeInTheDocument(),
      );
    });
  },
};

export const Horizontal: Story = {
  args: {
    children: (
      <>
        <PopoverTrigger>
          <Button>Click here!</Button>
        </PopoverTrigger>
        <PopoverContent horizontal="right">{content}</PopoverContent>
      </>
    ),
  },
  play: Example.play,
};

import { expect } from "@storybook/jest";
import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, waitFor, within } from "@storybook/testing-library";
import { InfoTooltip } from "./InfoTooltip";

const meta = {
  title: "components/InfoTooltip",
  component: InfoTooltip,
  args: {
    type: "info",
    title: "Hello, friend!",
    message: "Today is a lovely day :^)",
  },
} satisfies Meta<typeof InfoTooltip>;

export default meta;
type Story = StoryObj<typeof InfoTooltip>;

export const Example: Story = {
  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByRole("button"));
      await waitFor(() =>
        expect(screen.getByText(meta.args.message)).toBeInTheDocument(),
      );
    });
  },
};

export const Notice = {
  args: {
    type: "notice",
    message: "Unfortunately, there's a radio connected to my brain",
  },
  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByRole("button"));
      await waitFor(() =>
        expect(screen.getByText(Notice.args.message)).toBeInTheDocument(),
      );
    });
  },
} satisfies Story;

export const Warning = {
  args: {
    type: "warning",
    message: "Unfortunately, there's a radio connected to my brain",
  },
  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByRole("button"));
      await waitFor(() =>
        expect(screen.getByText(Warning.args.message)).toBeInTheDocument(),
      );
    });
  },
} satisfies Story;

import type { Meta, StoryObj } from "@storybook/react";
import { RuntimeErrorState } from "./RuntimeErrorState";

const error = new Error("An error occurred");

const meta: Meta<typeof RuntimeErrorState> = {
  title: "components/RuntimeErrorState",
  component: RuntimeErrorState,
  args: {
    error,
  },
  parameters: {
    // The RuntimeErrorState is noisy for chromatic, because it renders an actual error
    // along with the stacktrace - and the stacktrace includes the full URL of
    // scripts in the stack. This is problematic, because every deployment uses
    // a different URL, causing the validation to fail.
    chromatic: { disableSnapshot: true },
  },
};

export default meta;
type Story = StoryObj<typeof RuntimeErrorState>;

export const Errored: Story = {};

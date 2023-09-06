import { Story } from "@storybook/react";
import { RuntimeErrorState, RuntimeErrorStateProps } from "./RuntimeErrorState";

const error = new Error("An error occurred");

export default {
  title: "components/RuntimeErrorState",
  component: RuntimeErrorState,
  args: {
    error,
  },
};

const Template: Story<RuntimeErrorStateProps> = (args) => (
  <RuntimeErrorState {...args} />
);

export const Errored = Template.bind({});
Errored.parameters = {
  // The RuntimeErrorState is noisy for chromatic, because it renders an actual error
  // along with the stacktrace - and the stacktrace includes the full URL of
  // scripts in the stack. This is problematic, because every deployment uses
  // a different URL, causing the validation to fail.
  chromatic: { disableSnapshot: true },
};

Errored.args = {
  error,
};

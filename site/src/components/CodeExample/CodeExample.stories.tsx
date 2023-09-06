import { Story } from "@storybook/react";
import { CodeExample, CodeExampleProps } from "./CodeExample";

const sampleCode = `echo "Hello, world"`;

export default {
  title: "components/CodeExample",
  component: CodeExample,
  argTypes: {
    code: { control: "string", defaultValue: sampleCode },
  },
};

const Template: Story<CodeExampleProps> = (args: CodeExampleProps) => (
  <CodeExample {...args} />
);

export const Example = Template.bind({});
Example.args = {
  code: sampleCode,
};

export const LongCode = Template.bind({});
LongCode.args = {
  code: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICnKzATuWwmmt5+CKTPuRGN0R1PBemA+6/SStpLiyX+L",
};

import TextField from "@mui/material/TextField";
import { Story } from "@storybook/react";
import { Stack, StackProps } from "./Stack";

export default {
  title: "components/Stack",
  component: Stack,
};

const Template: Story<StackProps> = (args: StackProps) => (
  <Stack {...args}>
    <TextField autoFocus autoComplete="name" fullWidth label="Name" />
    <TextField autoComplete="email" fullWidth label="Email" />
    <TextField autoComplete="username" fullWidth label="Username" />
  </Stack>
);

export const Example = Template.bind({});
Example.args = {
  spacing: 2,
};

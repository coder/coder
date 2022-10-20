import TextField from "@material-ui/core/TextField"
import { Story } from "@storybook/react"
import { Stack, StackProps } from "./Stack"

export default {
  title: "components/Stack",
  component: Stack,
}

const Template: Story<StackProps> = (args: StackProps) => (
  <Stack {...args}>
    <TextField
      autoFocus
      autoComplete="name"
      fullWidth
      label="Name"
      variant="outlined"
    />
    <TextField
      autoComplete="email"
      fullWidth
      label="Email"
      variant="outlined"
    />
    <TextField
      autoComplete="username"
      fullWidth
      label="Username"
      variant="outlined"
    />
  </Stack>
)

export const Example = Template.bind({})
Example.args = {
  spacing: 2,
}

import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { Story } from "@storybook/react"
import { Section, SectionProps } from "./Section"

export default {
  title: "components/Section",
  component: Section,
  argTypes: {
    title: { type: "string" },
    description: { type: "string" },
    children: { control: { disable: true } },
  },
}

const Template: Story<SectionProps> = (args: SectionProps) => (
  <Section {...args} />
)

export const Example = Template.bind({})
Example.args = {
  title: "User Settings",
  description: "Add your personal info",
  children: (
    <>
      <form style={{ display: "grid", gridAutoFlow: "row", gap: 12 }}>
        <TextField label="Name" variant="filled" fullWidth />
        <TextField label="Email" variant="filled" fullWidth />
      </form>

      <Section.Action>
        <Button variant="contained" color="primary">
          Submit
        </Button>
      </Section.Action>
    </>
  ),
}

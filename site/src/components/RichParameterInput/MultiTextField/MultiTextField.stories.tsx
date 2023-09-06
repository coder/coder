import { Story } from "@storybook/react"
import { useState } from "react"
import { MultiTextField, MultiTextFieldProps } from "./MultiTextField"

export default {
  title: "components/MultiTextField",
  component: MultiTextField,
}

const Template: Story<MultiTextFieldProps> = (args) => {
  const [values, setValues] = useState(args.values ?? ["foo", "bar"])
  return <MultiTextField {...args} values={values} onChange={setValues} />
}

export const Example = Template.bind({})
Example.args = {}

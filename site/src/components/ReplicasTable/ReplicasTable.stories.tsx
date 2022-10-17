import { Story } from "@storybook/react"
import { ReplicasTable, ReplicasTableProps } from "./ReplicasTable"

export default {
  title: "components/ReplicasTable",
  component: ReplicasTable,
}

const Template: Story<ReplicasTableProps> = (args) => <ReplicasTable {...args} />

export const Single = Template.bind({})
Single.args = {
}

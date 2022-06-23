import { ComponentMeta, Story } from "@storybook/react"
import { workspaceFilterQuery } from "../../util/workspace"
import { SearchBarWithFilter, SearchBarWithFilterProps } from "./SearchBarWithFilter"

export default {
  title: "components/SearchBarWithFilter",
  component: SearchBarWithFilter,
  argTypes: {
    filter: {
      defaultValue: workspaceFilterQuery.me,
    },
  },
} as ComponentMeta<typeof SearchBarWithFilter>

const Template: Story<SearchBarWithFilterProps> = (args) => <SearchBarWithFilter {...args} />

export const WithoutPresetFilters = Template.bind({})

export const WithPresetFilters = Template.bind({})
WithPresetFilters.args = {
  ...WithoutPresetFilters.args,
  presetFilters: [
    { query: workspaceFilterQuery.me, name: "Your workspaces" },
    { query: "random query", name: "Random query" },
  ],
}

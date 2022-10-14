import { ComponentMeta, Story } from "@storybook/react"
import { userFilterQuery, workspaceFilterQuery } from "../../util/filters"
import {
  SearchBarWithFilter,
  SearchBarWithFilterProps,
} from "./SearchBarWithFilter"

export default {
  title: "components/SearchBarWithFilter",
  component: SearchBarWithFilter,
  argTypes: {
    filter: {
      defaultValue: workspaceFilterQuery.me,
    },
  },
} as ComponentMeta<typeof SearchBarWithFilter>

const Template: Story<SearchBarWithFilterProps> = (args) => (
  <SearchBarWithFilter {...args} />
)

export const WithoutPresetFilters = Template.bind({})

export const WithPresetFilters = Template.bind({})
WithPresetFilters.args = {
  presetFilters: [
    { query: workspaceFilterQuery.me, name: "Your workspaces" },
    { query: "random query", name: "Random query" },
  ],
}

export const WithError = Template.bind({})
WithError.args = {
  filter: "status:inactive",
  presetFilters: [
    { query: userFilterQuery.active, name: "Active users" },
    { query: "random query", name: "Random query" },
  ],
  error: {
    response: {
      data: {
        message: "Invalid user search query.",
        validations: [
          {
            field: "status",
            detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
          },
        ],
      },
    },
    isAxiosError: true,
  },
}

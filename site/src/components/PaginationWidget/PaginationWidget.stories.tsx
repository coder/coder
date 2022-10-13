import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { PaginationWidget, PaginationWidgetProps } from "./PaginationWidget"

export default {
  title: "components/PaginationWidget",
  component: PaginationWidget,
}

const Template: Story<PaginationWidgetProps> = (
  args: PaginationWidgetProps,
) => <PaginationWidget {...args} />

const defaultProps = {
  prevLabel: "Previous",
  nextLabel: "Next",
  onPrevClick: action("previous"),
  onNextClick: action("next"),
  onPageClick: action("clicked"),
}

export const UnknownPageNumbers = Template.bind({})
UnknownPageNumbers.args = {
  ...defaultProps,
}

export const LessThan8Pages = Template.bind({})
LessThan8Pages.args = {
  ...defaultProps,
  numRecords: 84,
  numRecordsPerPage: 12,
  activePage: 1,
}

export const MoreThan8Pages = Template.bind({})
MoreThan8Pages.args = {
  ...defaultProps,
  numRecords: 200,
  numRecordsPerPage: 12,
  activePage: 1,
}

export const MoreThan7PagesWithActivePageCloseToStart = Template.bind({})
MoreThan7PagesWithActivePageCloseToStart.args = {
  ...defaultProps,
  numRecords: 200,
  numRecordsPerPage: 12,
  activePage: 2,
}

export const MoreThan7PagesWithActivePageFarFromBoundaries = Template.bind({})
MoreThan7PagesWithActivePageFarFromBoundaries.args = {
  ...defaultProps,
  numRecords: 200,
  numRecordsPerPage: 12,
  activePage: 4,
}

export const MoreThan7PagesWithActivePageCloseToEnd = Template.bind({})
MoreThan7PagesWithActivePageCloseToEnd.args = {
  ...defaultProps,
  numRecords: 200,
  numRecordsPerPage: 12,
  activePage: 17,
}

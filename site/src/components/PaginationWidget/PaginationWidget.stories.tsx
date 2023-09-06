import { Story } from "@storybook/react";
import { PaginationWidget, PaginationWidgetProps } from "./PaginationWidget";
import { createPaginationRef } from "./utils";

export default {
  title: "components/PaginationWidget",
  component: PaginationWidget,
  args: {
    prevLabel: "Previous",
    nextLabel: "Next",
    paginationRef: createPaginationRef({ page: 1, limit: 12 }),
    numRecords: 200,
  },
};

const Template: Story<PaginationWidgetProps> = (
  args: PaginationWidgetProps,
) => <PaginationWidget {...args} />;

export const LessThan8Pages = Template.bind({});
LessThan8Pages.args = {
  numRecords: 84,
};

export const MoreThan8Pages = Template.bind({});

export const MoreThan7PagesWithActivePageCloseToStart = Template.bind({});
MoreThan7PagesWithActivePageCloseToStart.args = {
  paginationRef: createPaginationRef({ page: 2, limit: 12 }),
};

export const MoreThan7PagesWithActivePageFarFromBoundaries = Template.bind({});
MoreThan7PagesWithActivePageFarFromBoundaries.args = {
  paginationRef: createPaginationRef({ page: 4, limit: 12 }),
};

export const MoreThan7PagesWithActivePageCloseToEnd = Template.bind({});
MoreThan7PagesWithActivePageCloseToEnd.args = {
  paginationRef: createPaginationRef({ page: 17, limit: 12 }),
};

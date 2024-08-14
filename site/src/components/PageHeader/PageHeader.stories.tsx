import type { Meta, StoryObj } from "@storybook/react";
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "./PageHeader";

const meta: Meta<typeof PageHeader> = {
  title: "components/PageHeader",
  component: PageHeader,
};

export default meta;
type Story = StoryObj<typeof PageHeader>;

export const WithTitle: Story = {
  args: {
    children: <PageHeaderTitle>Templates</PageHeaderTitle>,
  },
};

export const WithSubtitle: Story = {
  args: {
    children: (
      <>
        <PageHeaderTitle>Templates</PageHeaderTitle>
        <PageHeaderSubtitle>
          Create a new workspace from a Template
        </PageHeaderSubtitle>
      </>
    ),
  },
};

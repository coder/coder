import { ComponentMeta, Story } from "@storybook/react";
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "./PageHeader";

export default {
  title: "components/PageHeader",
  component: PageHeader,
} as ComponentMeta<typeof PageHeader>;

const WithTitleTemplate: Story = () => (
  <PageHeader>
    <PageHeaderTitle>Templates</PageHeaderTitle>
  </PageHeader>
);

export const WithTitle = WithTitleTemplate.bind({});

const WithSubtitleTemplate: Story = () => (
  <PageHeader>
    <PageHeaderTitle>Templates</PageHeaderTitle>
    <PageHeaderSubtitle>
      Create a new workspace from a Template
    </PageHeaderSubtitle>
  </PageHeader>
);

export const WithSubtitle = WithSubtitleTemplate.bind({});

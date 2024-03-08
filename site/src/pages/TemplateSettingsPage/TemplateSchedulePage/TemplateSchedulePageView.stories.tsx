import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { MockTemplate } from "testHelpers/entities";
import { TemplateSchedulePageView } from "./TemplateSchedulePageView";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
      refetchOnWindowFocus: false,
      networkMode: "offlineFirst",
    },
  },
});

const meta: Meta<typeof TemplateSchedulePageView> = {
  title: "pages/TemplateSettingsPage/TemplateSchedulePageView",
  component: TemplateSchedulePageView,
  decorators: [
    (Story) => (
      <QueryClientProvider client={queryClient}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof TemplateSchedulePageView>;

const defaultArgs = {
  allowAdvancedScheduling: true,
  template: MockTemplate,
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
};

export const Example: Story = {
  args: { ...defaultArgs },
};

export const CantSetMaxTTL: Story = {
  args: { ...defaultArgs, allowAdvancedScheduling: false },
};

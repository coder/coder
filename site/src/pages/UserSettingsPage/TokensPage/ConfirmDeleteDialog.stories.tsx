import type { Meta, StoryObj } from "@storybook/react";
import { MockToken } from "testHelpers/entities";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { QueryClient, QueryClientProvider } from "react-query";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
      refetchOnWindowFocus: false,
    },
  },
});

const meta: Meta<typeof ConfirmDeleteDialog> = {
  title: "components/ConfirmDeleteDialog",
  component: ConfirmDeleteDialog,
  decorators: [
    (Story) => (
      <QueryClientProvider client={queryClient}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConfirmDeleteDialog>;

export const DeleteDialog: Story = {
  args: {
    queryKey: ["tokens"],
    token: MockToken,
    setToken: () => {
      return null;
    },
  },
};

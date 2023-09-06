import { Story } from "@storybook/react";
import { MockToken } from "testHelpers/entities";
import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogProps,
} from "./ConfirmDeleteDialog";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      cacheTime: 0,
      refetchOnWindowFocus: false,
    },
  },
});

export default {
  title: "components/ConfirmDeleteDialog",
  component: ConfirmDeleteDialog,
};

const Template: Story<ConfirmDeleteDialogProps> = (
  args: ConfirmDeleteDialogProps,
) => (
  <QueryClientProvider client={queryClient}>
    <ConfirmDeleteDialog {...args} />
  </QueryClientProvider>
);

export const DeleteDialog = Template.bind({});
DeleteDialog.args = {
  queryKey: ["tokens"],
  token: MockToken,
  setToken: () => {
    return null;
  },
};

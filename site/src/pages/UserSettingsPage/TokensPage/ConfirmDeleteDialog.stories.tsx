import { MockToken } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "react-query";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";

const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			retry: false,
			gcTime: 0,
			refetchOnWindowFocus: false,
		},
	},
});

const meta: Meta<typeof ConfirmDeleteDialog> = {
	title: "pages/UserSettingsPage/TokensDeleteDialog",
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
		setToken: () => null,
	},
};

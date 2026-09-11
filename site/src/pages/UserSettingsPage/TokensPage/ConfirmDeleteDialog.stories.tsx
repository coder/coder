import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "react-query";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { MockToken, mockApiError } from "#/testHelpers/entities";
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

export const FailedDelete: Story = {
	args: {
		queryKey: ["tokens"],
		token: MockToken,
		setToken: () => null,
	},
	beforeEach: () => {
		spyOn(API, "deleteToken").mockRejectedValue(
			mockApiError({
				message: "Failed to delete token.",
				detail: "The token is already expired.",
			}),
		);
	},
	play: async () => {
		const dialog = await within(document.body).findByRole("dialog");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Delete" }),
		);
		await within(dialog).findByRole("alert");
	},
};

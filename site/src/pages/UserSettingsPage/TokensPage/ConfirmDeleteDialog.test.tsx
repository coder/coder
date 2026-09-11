import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { API } from "#/api/api";
import { MockToken } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";

describe("ConfirmDeleteDialog", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("does not toast when deleting a token fails", async () => {
		const user = userEvent.setup({ delay: 0 });
		const toastError = vi.spyOn(toast, "error");
		const deleteTokenMock = vi
			.spyOn(API, "deleteToken")
			.mockRejectedValueOnce(new Error("Cannot delete token."));
		const setToken = vi.fn();

		renderWithAuth(
			<ConfirmDeleteDialog
				queryKey={["tokens"]}
				token={MockToken}
				setToken={setToken}
			/>,
		);

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));

		await waitFor(() => {
			expect(deleteTokenMock).toHaveBeenCalledWith(MockToken.id);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(setToken).not.toHaveBeenCalled();
	});
});

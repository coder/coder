import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { API } from "#/api/api";
import { MockUserMember, SuspendedMockUser } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { UserActionDialogs } from "./UserActionDialogs";

describe("UserActionDialogs", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("does not toast when suspending a user fails", async () => {
		const user = userEvent.setup({ delay: 0 });
		const toastError = vi.spyOn(toast, "error");
		const suspendUserMock = vi
			.spyOn(API, "suspendUser")
			.mockRejectedValueOnce(new Error("Cannot suspend user."));
		const onClose = vi.fn();

		renderWithAuth(
			<UserActionDialogs
				action={{ type: "suspend", user: MockUserMember }}
				onClose={onClose}
			/>,
		);

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Suspend" }));

		await waitFor(() => {
			expect(suspendUserMock).toHaveBeenCalledWith(MockUserMember.id);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
	});

	it("does not toast when deleting a user fails", async () => {
		const user = userEvent.setup({ delay: 0 });
		const toastError = vi.spyOn(toast, "error");
		const deleteUserMock = vi
			.spyOn(API, "deleteUser")
			.mockRejectedValueOnce(new Error("Cannot delete user."));
		const onClose = vi.fn();
		const onDeleted = vi.fn();

		renderWithAuth(
			<UserActionDialogs
				action={{ type: "delete", user: MockUserMember }}
				onClose={onClose}
				onDeleted={onDeleted}
			/>,
		);

		const dialog = await screen.findByRole("dialog");
		await user.type(
			within(dialog).getByLabelText("Name of the user to delete"),
			MockUserMember.username,
		);
		await user.click(within(dialog).getByRole("button", { name: "Delete" }));

		await waitFor(() => {
			expect(deleteUserMock).toHaveBeenCalledWith(MockUserMember.id);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
		expect(onDeleted).not.toHaveBeenCalled();
	});

	it("does not toast when activating a user fails", async () => {
		const user = userEvent.setup({ delay: 0 });
		const toastError = vi.spyOn(toast, "error");
		const activateUserMock = vi
			.spyOn(API, "activateUser")
			.mockRejectedValueOnce(new Error("Cannot activate user."));
		const onClose = vi.fn();

		renderWithAuth(
			<UserActionDialogs
				action={{ type: "activate", user: SuspendedMockUser }}
				onClose={onClose}
			/>,
		);

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Activate" }));

		await waitFor(() => {
			expect(activateUserMock).toHaveBeenCalledWith(SuspendedMockUser.id);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
	});
});

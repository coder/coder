import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockGitSSHKey,
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import SSHKeysPage from "./SSHKeysPage";

describe("SSH keys Page", () => {
	it("shows the SSH key", async () => {
		renderWithAuth(<SSHKeysPage />);
		await screen.findByText(MockGitSSHKey.public_key);
	});

	it("enables commit signing", async () => {
		vi.spyOn(API, "getExperiments").mockResolvedValue(["git-commit-signing"]);
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		const update = vi
			.spyOn(API, "updateUserPreferenceSettings")
			.mockResolvedValueOnce({
				...MockUserPreferenceSettings,
				sign_commits_with_ssh_key: true,
			});
		renderWithAuth(<SSHKeysPage />);

		const checkbox = await screen.findByRole("checkbox", {
			name: "Sign my commits with my Coder SSH key",
		});
		await waitFor(() => expect(checkbox).toBeEnabled());
		await userEvent.click(checkbox);

		await waitFor(() =>
			expect(update).toHaveBeenCalledWith({ sign_commits_with_ssh_key: true }),
		);
	});

	describe("regenerate SSH key", () => {
		describe("when it is success", () => {
			it("shows a success message and updates the ssh key on the page", async () => {
				renderWithAuth(<SSHKeysPage />);

				// Wait to the ssh be rendered on the screen
				await screen.findByText(MockGitSSHKey.public_key);

				// Click on the "Regenerate" button to display the confirm dialog
				const regenerateButton = screen.getByTestId("regenerate");
				fireEvent.click(regenerateButton);
				const confirmDialog = screen.getByRole("dialog");
				expect(confirmDialog).toHaveTextContent(
					"You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
				);

				const newUserSSHKey =
					"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDSC/ouD/LqiT1Rd99vDv/MwUmqzJuinLTMTpk5kVy66";
				vi.spyOn(API, "regenerateUserSSHKey").mockResolvedValueOnce({
					...MockGitSSHKey,
					public_key: newUserSSHKey,
				});

				// Click on the "Confirm" button
				const confirmButton = within(confirmDialog).getByRole("button", {
					name: "Confirm",
				});
				fireEvent.click(confirmButton);

				// Check if the success message is displayed
				await screen.findByText("SSH Key regenerated successfully.");

				// Check if the API was called correctly
				expect(API.regenerateUserSSHKey).toBeCalledTimes(1);

				// Check if the SSH key is updated
				await screen.findByText(newUserSSHKey);
			});
		});

		describe("when it fails", () => {
			it("shows an error message", async () => {
				renderWithAuth(<SSHKeysPage />);

				// Wait to the ssh be rendered on the screen
				await screen.findByText(MockGitSSHKey.public_key);

				vi.spyOn(API, "regenerateUserSSHKey").mockRejectedValueOnce(
					mockApiError({
						message: "Failed to regenerate SSH key",
					}),
				);

				// Click on the "Regenerate" button to display the confirm dialog
				const regenerateButton = screen.getByTestId("regenerate");
				fireEvent.click(regenerateButton);
				const confirmDialog = screen.getByRole("dialog");
				expect(confirmDialog).toHaveTextContent(
					"You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
				);

				// Click on the "Confirm" button
				const confirmButton = within(confirmDialog).getByRole("button", {
					name: "Confirm",
				});
				fireEvent.click(confirmButton);

				// Check if the error message is displayed
				await screen.findByText("Failed to regenerate SSH key");

				// Check if the API was called correctly
				expect(API.regenerateUserSSHKey).toBeCalledTimes(1);
			});
		});
	});
});

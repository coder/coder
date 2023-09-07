import { fireEvent, screen, within } from "@testing-library/react";
import * as API from "../../../api/api";
import { renderWithAuth } from "../../../testHelpers/renderHelpers";
import { Language as SSHKeysPageLanguage, SSHKeysPage } from "./SSHKeysPage";
import { Language as SSHKeysPageViewLanguage } from "./SSHKeysPageView";
import { i18n } from "i18n";
import { MockGitSSHKey, mockApiError } from "testHelpers/entities";

const { t } = i18n;

describe("SSH keys Page", () => {
  it("shows the SSH key", async () => {
    renderWithAuth(<SSHKeysPage />);
    await screen.findByText(MockGitSSHKey.public_key);
  });

  describe("regenerate SSH key", () => {
    describe("when it is success", () => {
      it("shows a success message and updates the ssh key on the page", async () => {
        renderWithAuth(<SSHKeysPage />);

        // Wait to the ssh be rendered on the screen
        await screen.findByText(MockGitSSHKey.public_key);

        // Click on the "Regenerate" button to display the confirm dialog
        const regenerateButton = screen.getByRole("button", {
          name: SSHKeysPageViewLanguage.regenerateLabel,
        });
        fireEvent.click(regenerateButton);
        const confirmDialog = screen.getByRole("dialog");
        expect(confirmDialog).toHaveTextContent(
          SSHKeysPageLanguage.regenerateDialogMessage,
        );

        const newUserSSHKey =
          "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDSC/ouD/LqiT1Rd99vDv/MwUmqzJuinLTMTpk5kVy66";
        jest.spyOn(API, "regenerateUserSSHKey").mockResolvedValueOnce({
          ...MockGitSSHKey,
          public_key: newUserSSHKey,
        });

        // Click on the "Confirm" button
        const confirmButton = within(confirmDialog).getByRole("button", {
          name: SSHKeysPageLanguage.confirmLabel,
        });
        fireEvent.click(confirmButton);

        // Check if the success message is displayed
        const successMessage = t("sshRegenerateSuccessMessage", {
          ns: "userSettingsPage",
        });
        await screen.findByText(successMessage);

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

        jest.spyOn(API, "regenerateUserSSHKey").mockRejectedValueOnce(
          mockApiError({
            message: "Error regenerating SSH key",
          }),
        );

        // Click on the "Regenerate" button to display the confirm dialog
        const regenerateButton = screen.getByRole("button", {
          name: SSHKeysPageViewLanguage.regenerateLabel,
        });
        fireEvent.click(regenerateButton);
        const confirmDialog = screen.getByRole("dialog");
        expect(confirmDialog).toHaveTextContent(
          SSHKeysPageLanguage.regenerateDialogMessage,
        );

        // Click on the "Confirm" button
        const confirmButton = within(confirmDialog).getByRole("button", {
          name: SSHKeysPageLanguage.confirmLabel,
        });
        fireEvent.click(confirmButton);

        // Check if the error message is displayed
        await screen.findByText("Error regenerating SSH key");

        // Check if the API was called correctly
        expect(API.regenerateUserSSHKey).toBeCalledTimes(1);
      });
    });
  });
});

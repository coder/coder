import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { OAuthConversionResponse } from "#/api/typesGenerated";
import { MockAuthMethodsAll, mockApiError } from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import SecurityPage from "./SecurityPage";
import * as SSO from "./SingleSignOnSection";

const renderPage = async () => {
	const utils = renderWithAuth(<SecurityPage />);
	await waitForLoaderToBeRemoved();
	return utils;
};

const newSecurityFormValues = {
	old_password: "password1",
	password: "password2",
	confirm_password: "password2",
};

const fillAndSubmitSecurityForm = () => {
	fireEvent.change(screen.getByLabelText("Old Password"), {
		target: { value: newSecurityFormValues.old_password },
	});
	fireEvent.change(screen.getByLabelText("New Password"), {
		target: { value: newSecurityFormValues.password },
	});
	fireEvent.change(screen.getByLabelText("Confirm Password"), {
		target: { value: newSecurityFormValues.confirm_password },
	});
	fireEvent.click(screen.getByText("Update password"));
};

beforeEach(() => {
	vi.spyOn(API, "getAuthMethods").mockResolvedValue(MockAuthMethodsAll);
	vi.spyOn(API, "getUserLoginType").mockResolvedValue({
		login_type: "password",
	});
});

test("update password successfully", async () => {
	vi.spyOn(API, "updateUserPassword").mockImplementationOnce((_userId, _data) =>
		Promise.resolve(undefined),
	);
	const { user } = await renderPage();
	fillAndSubmitSecurityForm();

	const successMessage = await screen.findByText("Updated password.");
	expect(successMessage).toBeDefined();
	expect(API.updateUserPassword).toBeCalledTimes(1);
	expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues);

	await waitFor(() => expect(location.pathname).toBe("/"));
});

test("update password with incorrect old password", async () => {
	vi.spyOn(API, "updateUserPassword").mockRejectedValueOnce(
		mockApiError({
			message: "Incorrect password.",
			validations: [{ detail: "Incorrect password.", field: "old_password" }],
		}),
	);

	const { user } = await renderPage();
	fillAndSubmitSecurityForm();

	const errorMessage = await screen.findAllByText("Incorrect password.");
	expect(errorMessage).toBeDefined();
	expect(errorMessage).toHaveLength(2);
	expect(API.updateUserPassword).toBeCalledTimes(1);
	expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues);
});

test("update password when submit returns an unknown error", async () => {
	vi.spyOn(API, "updateUserPassword").mockRejectedValueOnce({
		data: "unknown error",
	});

	const { user } = await renderPage();
	fillAndSubmitSecurityForm();

	const errorMessage = await screen.findByText("Something went wrong.");
	expect(errorMessage).toBeDefined();
	expect(API.updateUserPassword).toBeCalledTimes(1);
	expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues);
});

test("change login type to OIDC", async () => {
	const user = userEvent.setup();
	const { user: userData } = await renderPage();
	const convertToOAUTHSpy = vi.spyOn(API, "convertToOAUTH").mockResolvedValue({
		state_string: "some-state-string",
		expires_at: "2021-01-01T00:00:00Z",
		to_type: "oidc",
		user_id: userData.id,
	} as OAuthConversionResponse);

	vi.spyOn(SSO, "redirectToOIDCAuth").mockImplementation(() => {
		// Does a noop
		return "";
	});

	const ssoSection = screen.getByTestId("sso-section");
	const githubButton = within(ssoSection).getByText("GitHub", { exact: false });
	await user.click(githubButton);

	const confirmationDialog = await screen.findByTestId("dialog");
	const confirmPasswordField = within(confirmationDialog).getByLabelText(
		"Confirm your password",
	);
	await user.type(confirmPasswordField, "password123");
	const updateButton = within(confirmationDialog).getByText("Update");
	await user.click(updateButton);

	await waitFor(() => {
		expect(convertToOAUTHSpy).toHaveBeenCalledWith({
			password: "password123",
			to_type: "github",
		});
	});
});

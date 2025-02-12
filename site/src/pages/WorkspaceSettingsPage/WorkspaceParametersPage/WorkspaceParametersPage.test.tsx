import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import {
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter4,
	MockWorkspace,
	MockWorkspaceBuild,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter4,
} from "testHelpers/entities";
import {
	renderWithWorkspaceSettingsLayout,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import WorkspaceParametersPage from "./WorkspaceParametersPage";

test("Submit the workspace settings page successfully", async () => {
	// Mock the API calls that loads data
	jest
		.spyOn(API, "getWorkspaceByOwnerAndName")
		.mockResolvedValueOnce(MockWorkspace);
	jest.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
		MockTemplateVersionParameter1,
		MockTemplateVersionParameter2,
		// Immutable parameters
		MockTemplateVersionParameter4,
	]);
	jest.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([
		MockWorkspaceBuildParameter1,
		MockWorkspaceBuildParameter2,
		// Immutable value
		MockWorkspaceBuildParameter4,
	]);
	// Mock the API calls that submit data
	const postWorkspaceBuildSpy = jest
		.spyOn(API, "postWorkspaceBuild")
		.mockResolvedValue(MockWorkspaceBuild);
	// Setup event and rendering
	const user = userEvent.setup();
	renderWithWorkspaceSettingsLayout(<WorkspaceParametersPage />, {
		route: "/@test-user/test-workspace/settings",
		path: "/:username/:workspace/settings",
		// Need this because after submit the user is redirected
		extraRoutes: [{ path: "/:username/:workspace", element: <div /> }],
	});
	await waitForLoaderToBeRemoved();
	// Fill the form and submit
	const form = screen.getByTestId("form");
	const parameter1 = within(form).getByLabelText(
		MockWorkspaceBuildParameter1.name,
		{ exact: false },
	);
	await user.clear(parameter1);
	await user.type(parameter1, "new-value");
	const parameter2 = within(form).getByLabelText(
		MockWorkspaceBuildParameter2.name,
		{ exact: false },
	);
	await user.clear(parameter2);
	await user.type(parameter2, "3");
	await user.click(
		within(form).getByRole("button", { name: "Submit and restart" }),
	);
	// Assert that the API calls were made with the correct data
	await waitFor(() => {
		expect(postWorkspaceBuildSpy).toHaveBeenCalledWith(MockWorkspace.id, {
			transition: "start",
			rich_parameter_values: [
				{ name: MockTemplateVersionParameter1.name, value: "new-value" },
				{ name: MockTemplateVersionParameter2.name, value: "3" },
			],
		});
	});
});

test("Submit button is only enabled when changes are made", async () => {
	// Mock the API calls that loads data
	jest
		.spyOn(API, "getWorkspaceByOwnerAndName")
		.mockResolvedValueOnce(MockWorkspace);
	jest.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
		MockTemplateVersionParameter1,
		MockTemplateVersionParameter2,
		// Immutable parameters
		MockTemplateVersionParameter4,
	]);
	jest.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([
		MockWorkspaceBuildParameter1,
		MockWorkspaceBuildParameter2,
		// Immutable value
		MockWorkspaceBuildParameter4,
	]);
	// Setup event and rendering
	const user = userEvent.setup();
	renderWithWorkspaceSettingsLayout(<WorkspaceParametersPage />, {
		route: "/@test-user/test-workspace/settings",
		path: "/:username/:workspace/settings",
		// Need this because after submit the user is redirected
		extraRoutes: [{ path: "/:username/:workspace", element: <div /> }],
	});
	await waitForLoaderToBeRemoved();

	const submitButton: HTMLButtonElement = screen.getByRole("button", {
		name: "Submit and restart",
	});

	const form = screen.getByTestId("form");
	const parameter1 = within(form).getByLabelText(
		MockWorkspaceBuildParameter1.name,
		{ exact: false },
	);

	// There are no changes, the button should be disabled.
	expect(submitButton.disabled).toBeTruthy();

	// Make changes to the form
	await user.clear(parameter1);
	await user.type(parameter1, "new-value");

	// There are now changes, the button should be enabled.
	expect(submitButton.disabled).toBeFalsy();

	// Change form value back to default
	await user.clear(parameter1);
	await user.type(parameter1, MockWorkspaceBuildParameter1.value);

	// There are now no changes, the button should be disabled.
	expect(submitButton.disabled).toBeTruthy();
});

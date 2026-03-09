import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";

// Default props matching the stories configuration.
const defaultProps = {
	autofillParameters: [],
	creatingWorkspace: false,
	defaultOwner: MockUserOwner,
	diagnostics: [],
	error: undefined,
	externalAuth: [],
	externalAuthPollingState: "idle" as const,
	hasAllRequiredExternalAuth: true,
	mode: "form" as const,
	parameters: [],
	permissions: {
		createWorkspaceForAny: true,
		canUpdateTemplate: false,
	},
	presets: [],
	template: MockTemplate,
	onCancel: () => {},
	onSubmit: () => {},
	resetMutation: () => {},
	sendMessage: () => {},
	startPollingExternalAuth: () => {},
	owner: MockUserOwner,
	setOwner: () => {},
};

test("shows inline validation error immediately when URL-prefilled name exceeds 32 characters", async () => {
	render(
		<CreateWorkspacePageView
			{...defaultProps}
			defaultName="this-name-is-way-too-long-and-exceeds-the-limit"
		/>,
	);

	// The error should be visible without any user interaction since
	// the name field is marked as initially touched when pre-filled.
	const error = await screen.findByText(
		/Workspace Name cannot be longer than 32 characters/i,
	);
	expect(error).toBeVisible();
});

test("shows no validation error when URL-prefilled name is valid", async () => {
	render(
		<CreateWorkspacePageView {...defaultProps} defaultName="valid-name" />,
	);

	// Wait for the form to settle, then confirm no length error appears.
	// Use the workspace name input as a proxy that the form rendered.
	await screen.findByLabelText(/Workspace Name/i);

	expect(
		screen.queryByText(/Workspace Name cannot be longer than 32 characters/i),
	).not.toBeInTheDocument();
});

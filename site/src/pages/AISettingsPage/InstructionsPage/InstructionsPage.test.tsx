import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { organizationChatSystemPromptKey } from "#/api/queries/chats";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockOrganization2,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import InstructionsPage from "./InstructionsPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: MockNoPermissions,
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization, MockOrganization2],
	}),
}));

afterEach(() => {
	vi.restoreAllMocks();
});

const mockInstructionsApi = ({
	canEdit = true,
	readableOrganizations = [MockDefaultOrganization, MockOrganization2],
} = {}) => {
	vi.spyOn(API, "checkAuthorization").mockResolvedValue(
		Object.fromEntries(
			readableOrganizations.flatMap((organization) => [
				[`${organization.id}.viewChatModelConfigs`, true],
				[`${organization.id}.editChatModelConfigs`, canEdit],
			]),
		),
	);
	vi.spyOn(
		API.experimental,
		"getOrganizationChatSystemPrompt",
	).mockImplementation(async (organizationId) => ({
		system_prompt:
			organizationId === MockOrganization2.id
				? "Second organization guidance."
				: "Default organization guidance.",
	}));
	return vi
		.spyOn(API.experimental, "updateOrganizationChatSystemPrompt")
		.mockResolvedValue();
};

const renderPage = () =>
	renderWithRouter(
		createMemoryRouter(
			[{ path: "/ai/settings/instructions", element: <InstructionsPage /> }],
			{ initialEntries: ["/ai/settings/instructions"] },
		),
	);

it("saves edits to the organization picked in the picker", async () => {
	const updateSystemPrompt = mockInstructionsApi();
	const user = userEvent.setup();
	renderPage();

	await user.type(
		await screen.findByDisplayValue("Default organization guidance."),
		" Unsaved.",
	);
	await user.click(
		screen.getByRole("combobox", {
			name: `Organization ${MockDefaultOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);
	await user.type(
		await screen.findByDisplayValue("Second organization guidance."),
		" Edited.",
	);
	await user.click(screen.getByRole("button", { name: "Save" }));

	await waitFor(() =>
		expect(updateSystemPrompt).toHaveBeenCalledWith(MockOrganization2.id, {
			system_prompt: "Second organization guidance. Edited.",
		}),
	);
});

it("opens an organization whose instructions the user can read", async () => {
	mockInstructionsApi({ readableOrganizations: [MockOrganization2] });
	renderPage();

	await waitFor(() =>
		expect(
			API.experimental.getOrganizationChatSystemPrompt,
		).toHaveBeenCalledWith(MockOrganization2.id),
	);
	expect(
		API.experimental.getOrganizationChatSystemPrompt,
	).not.toHaveBeenCalledWith(MockDefaultOrganization.id);
});

it("keeps the instructions read-only for viewers", async () => {
	mockInstructionsApi({ canEdit: false });
	const user = userEvent.setup();
	renderPage();

	const field = await screen.findByDisplayValue(
		"Default organization guidance.",
	);
	await user.type(field, " Edited.");

	expect(field).toHaveValue("Default organization guidance.");
});

it("keeps an unsaved edit when a background refetch fails", async () => {
	const updateSystemPrompt = mockInstructionsApi();
	const user = userEvent.setup();
	const { queryClient } = renderPage();

	await user.type(
		await screen.findByDisplayValue("Default organization guidance."),
		" Edited.",
	);
	vi.mocked(API.experimental.getOrganizationChatSystemPrompt).mockRejectedValue(
		new Error("prompt unavailable"),
	);
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationChatSystemPromptKey(MockDefaultOrganization.id),
		}),
	);
	await screen.findByText("prompt unavailable");

	await user.click(screen.getByRole("button", { name: "Save" }));
	await waitFor(() =>
		expect(updateSystemPrompt).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			{ system_prompt: "Default organization guidance. Edited." },
		),
	);
});

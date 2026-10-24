import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockGitHubRunnerPool,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import GitHubRunnersPage from "./GitHubRunnersPage";

beforeEach(() => {
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API, "getGitHubRunnerPools").mockResolvedValue([
		MockGitHubRunnerPool,
	]);
});

test("switches the pool query to the selected organization", async () => {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
	]);
	const { router } = renderWithAuth(<GitHubRunnersPage />);
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("combobox", { name: "Organization" }),
	);
	await user.click(
		screen.getByRole("option", { name: MockOrganization2.display_name }),
	);
	await waitFor(() =>
		expect(API.getGitHubRunnerPools).toHaveBeenLastCalledWith(
			MockOrganization2.id,
		),
	);
	expect(
		new URLSearchParams(router.state.location.search).get("organization"),
	).toBe(MockOrganization2.id);
});

test("does not request pools when organization access is denied", async () => {
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(
			Object.entries(checks).map(([key, check]) => [
				key,
				check.object.resource_type !== "github_runner_pool",
			]),
		),
	);
	renderWithAuth(<GitHubRunnersPage />);
	await screen.findByText(
		"You do not have permission to view GitHub runner pools in any organization.",
	);
	expect(API.getGitHubRunnerPools).not.toHaveBeenCalled();
});

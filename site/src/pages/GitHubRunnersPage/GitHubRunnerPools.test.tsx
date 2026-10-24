import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { githubRunnerPoolsKey } from "#/api/queries/githubRunners";
import {
	MockDefaultOrganization,
	MockGitHubRunner,
	MockGitHubRunnerPool,
	MockNoOrganizationPermissions,
	MockOrganizationPermissions,
	MockTemplate,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { GitHubRunnerPools } from "./GitHubRunnerPools";

beforeEach(() => {
	vi.spyOn(API, "getGitHubRunnerPools").mockResolvedValue([
		MockGitHubRunnerPool,
	]);
	vi.spyOn(API, "getGitHubRunners").mockResolvedValue([MockGitHubRunner]);
	vi.spyOn(API, "getTemplatesByOrganization").mockResolvedValue([MockTemplate]);
});

const renderPools = (permissions = MockOrganizationPermissions) =>
	renderWithAuth(
		<GitHubRunnerPools
			organization={MockDefaultOrganization}
			permissions={permissions}
		/>,
	);

test("creates a pool with a write-only token and selected template", async () => {
	const create = vi
		.spyOn(API, "createGitHubRunnerPool")
		.mockResolvedValue(MockGitHubRunnerPool);
	renderPools();
	const user = userEvent.setup();
	await user.click(await screen.findByRole("button", { name: "Create pool" }));
	const dialog = within(screen.getByRole("dialog"));
	await user.type(dialog.getByLabelText(/Pool name/), "new-linux-pool");
	await user.type(
		dialog.getByLabelText(/GitHub URL/),
		MockGitHubRunnerPool.github_config_url,
	);
	await user.type(dialog.getByLabelText(/GitHub token/), "test-runner-token");
	await user.click(dialog.getByRole("combobox", { name: /Runner template/ }));
	await user.click(
		await screen.findByRole("option", {
			name: MockTemplate.display_name || MockTemplate.name,
		}),
	);
	await user.click(dialog.getByRole("button", { name: "Create pool" }));
	await waitFor(() =>
		expect(create).toHaveBeenCalledWith(MockDefaultOrganization.id, {
			name: "new-linux-pool",
			template_id: MockTemplate.id,
			owner_id: MockUserOwner.id,
			github_config_url: MockGitHubRunnerPool.github_config_url,
			github_token: "test-runner-token",
			runner_group_id: 1,
			max_runners: 2,
			startup_timeout_seconds: 600,
			job_timeout_seconds: 21600,
		}),
	);
});

test.each([
	["active", "Pause", "paused"],
	["paused", "Resume", "active"],
])(
	"changes %s pools through the management menu",
	async (state, label, nextState) => {
		vi.spyOn(API, "getGitHubRunnerPools").mockResolvedValue([
			{ ...MockGitHubRunnerPool, state },
		]);
		const update = vi
			.spyOn(API, "updateGitHubRunnerPool")
			.mockResolvedValue({ ...MockGitHubRunnerPool, state: nextState });
		renderPools();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: `Manage ${MockGitHubRunnerPool.name}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: label }));
		await waitFor(() =>
			expect(update).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				MockGitHubRunnerPool.id,
				{ state: nextState },
			),
		);
	},
);

test("keeps capacity edits when the pool refetches and invalidates after saving", async () => {
	const update = vi
		.spyOn(API, "updateGitHubRunnerPool")
		.mockResolvedValue({ ...MockGitHubRunnerPool, max_runners: 4 });
	const { queryClient } = renderPools();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: `Manage ${MockGitHubRunnerPool.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "Edit capacity" }));
	const input = screen.getByRole("spinbutton", { name: /Maximum runners/ });
	await user.clear(input);
	await user.type(input, "4");
	queryClient.setQueryData(githubRunnerPoolsKey(MockDefaultOrganization.id), [
		{ ...MockGitHubRunnerPool, max_runners: 9 },
	]);
	await user.click(screen.getByRole("button", { name: "Save capacity" }));
	await waitFor(() =>
		expect(update).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			MockGitHubRunnerPool.id,
			{ max_runners: 4 },
		),
	);
	await waitFor(() =>
		expect(API.getGitHubRunnerPools).toHaveBeenCalledTimes(2),
	);
});

test("requires confirmation to drain and delete a pool", async () => {
	const remove = vi.spyOn(API, "deleteGitHubRunnerPool").mockResolvedValue();
	renderPools();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: `Manage ${MockGitHubRunnerPool.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "Drain and delete" }));
	expect(remove).not.toHaveBeenCalled();
	await user.click(
		within(screen.getByRole("dialog")).getByRole("button", {
			name: "Drain and delete",
		}),
	);
	await waitFor(() =>
		expect(remove).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			MockGitHubRunnerPool.id,
		),
	);
});

test("lets a read-only viewer inspect the pool's runner allocations", async () => {
	renderPools({
		...MockNoOrganizationPermissions,
		viewGitHubRunnerPools: true,
	});
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: MockGitHubRunnerPool.name }),
	);
	await waitFor(() =>
		expect(API.getGitHubRunners).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			MockGitHubRunnerPool.id,
		),
	);
});

test("retries a failed pool request", async () => {
	vi.spyOn(API, "getGitHubRunnerPools").mockRejectedValueOnce(
		new Error("Runner pools are unavailable."),
	);
	renderPools();
	const user = userEvent.setup();
	await user.click(await screen.findByRole("button", { name: "Retry pools" }));
	await waitFor(() =>
		expect(API.getGitHubRunnerPools).toHaveBeenCalledTimes(2),
	);
});

test("rejects invalid capacity without submitting a request", async () => {
	const update = vi
		.spyOn(API, "updateGitHubRunnerPool")
		.mockResolvedValue(MockGitHubRunnerPool);
	renderPools();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: `Manage ${MockGitHubRunnerPool.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "Edit capacity" }));
	const input = screen.getByRole("spinbutton", { name: /Maximum runners/ });
	await user.clear(input);
	await user.type(input, "0");
	await user.click(screen.getByRole("button", { name: "Save capacity" }));
	expect(update).not.toHaveBeenCalled();
});

test.each(["Edit capacity", "Drain and delete"])(
	"restores focus after closing %s opened from the keyboard",
	async (action) => {
		renderPools();
		const user = userEvent.setup();
		const trigger = await screen.findByRole("button", {
			name: `Manage ${MockGitHubRunnerPool.name}`,
		});
		trigger.focus();
		await user.keyboard("{Enter}");
		await user.click(screen.getByRole("menuitem", { name: action }));
		await user.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Cancel",
			}),
		);
		await waitFor(() => expect(trigger).toHaveFocus());
	},
);

test("retries a failed capacity update without discarding the entered value", async () => {
	const update = vi
		.spyOn(API, "updateGitHubRunnerPool")
		.mockRejectedValueOnce(new Error("Capacity update failed."))
		.mockResolvedValue(MockGitHubRunnerPool);
	renderPools();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: `Manage ${MockGitHubRunnerPool.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "Edit capacity" }));
	const input = screen.getByRole("spinbutton", { name: /Maximum runners/ });
	await user.clear(input);
	await user.type(input, "5");
	await user.click(screen.getByRole("button", { name: "Save capacity" }));
	await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
	await user.click(
		await screen.findByRole("button", { name: "Save capacity" }),
	);
	await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
	expect(update).toHaveBeenLastCalledWith(
		MockDefaultOrganization.id,
		MockGitHubRunnerPool.id,
		{ max_runners: 5 },
	);
});

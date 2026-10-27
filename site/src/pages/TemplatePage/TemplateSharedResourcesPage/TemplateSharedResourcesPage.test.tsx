import "#/testHelpers/monaco";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import {
	MockTemplate,
	MockTemplateVersion,
	mockApiError,
} from "#/testHelpers/entities";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
import {
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
	MockTemplateResources,
} from "#/testHelpers/templateResources";
import TemplateSharedResourcesPage from "./TemplateSharedResourcesPage";

vi.mock(
	import("#/pages/TemplatePage/TemplateLayout"),
	async (importOriginal) => ({
		...(await importOriginal()),
		useTemplateLayoutContext: () => ({
			template: MockTemplate,
			activeVersion: MockTemplateVersion,
			permissions: { canUpdateTemplate: true },
		}),
	}),
);

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ experiments: ["template-resources"] }),
}));

beforeEach(() => {
	vi.spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
		MockTemplateResourceLog,
	]);
});

it("opens the full archive editor instead of saving a main.tf-only revision", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getTemplateResources").mockResolvedValue(
		MockTemplateResources,
	);
	const save = vi.spyOn(API, "createTemplateResourceRevision");
	const { router } = renderWithRouter(
		createMemoryRouter([
			{ path: "/", element: <TemplateSharedResourcesPage /> },
			{ path: "/templates/:template/edit", element: <div /> },
		]),
	);
	await user.click(await screen.findByRole("link", { name: "Edit files" }));
	expect(router.state.location.pathname).toBe(
		`/templates/${MockTemplate.name}/edit`,
	);
	expect(router.state.location.search).toBe("?scope=template");
	expect(save).not.toHaveBeenCalled();
});

it("retries a failed apply without creating another revision", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getTemplateResources").mockResolvedValue(
		MockTemplateResources,
	);
	const save = vi.spyOn(API, "createTemplateResourceRevision");
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockRejectedValueOnce(
			mockApiError({ message: "No provisioner available." }),
		)
		.mockResolvedValue(MockTemplateResourceRun);
	const { queryClient } = render(<TemplateSharedResourcesPage />);
	await user.click(await screen.findByRole("button", { name: "Apply" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
	expect(run).toHaveBeenLastCalledWith(MockTemplate.id, {
		revision_id: MockTemplateResourceRevision.id,
		action: "apply",
	});
	expect(save).not.toHaveBeenCalled();
});

it("plans the selected historical revision and destroys the applied revision only after confirmation", async () => {
	const user = userEvent.setup();
	const mockHistorical = {
		...MockTemplateResourceRevision,
		id: "70000000-0000-4000-8000-000000000001",
		created_at: "2026-10-20T12:00:00Z",
	};
	vi.spyOn(API, "getTemplateResources").mockResolvedValue({
		...MockTemplateResources,
		revisions: [mockHistorical, MockTemplateResourceRevision],
	});
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockResolvedValue(MockTemplateResourceRun);
	const { queryClient } = render(<TemplateSharedResourcesPage />);
	await user.click(
		await screen.findByRole("combobox", { name: "Revision history" }),
	);
	await user.click(screen.getByRole("option", { name: /70000000/ }));
	await user.click(screen.getByRole("button", { name: "Plan" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(MockTemplate.id, {
			revision_id: mockHistorical.id,
			action: "plan",
		}),
	);
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await user.click(screen.getByRole("button", { name: "Destroy" }));
	expect(run).toHaveBeenCalledTimes(1);
	await user.click(
		within(
			screen.getByRole("dialog", { name: "Destroy shared resources?" }),
		).getByRole("button", { name: "Destroy shared resources" }),
	);
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(MockTemplate.id, {
			revision_id: MockTemplateResourceRevision.id,
			action: "destroy",
		}),
	);
});

it("fetches logs for the selected historical run", async () => {
	const user = userEvent.setup();
	const mockHistorical = {
		...MockTemplateResourceRun,
		id: "older-run",
		created_at: "2026-10-20T12:00:00Z",
	};
	vi.spyOn(API, "getTemplateResources").mockResolvedValue({
		...MockTemplateResources,
		runs: [mockHistorical, MockTemplateResourceRun],
	});
	vi.mocked(API.getTemplateResourceRunLogs).mockRejectedValueOnce(
		mockApiError({ message: "Unable to load job logs." }),
	);
	render(<TemplateSharedResourcesPage />);
	await user.click(await screen.findByRole("button", { name: "Retry logs" }));
	await waitFor(() =>
		expect(API.getTemplateResourceRunLogs).toHaveBeenCalledTimes(2),
	);
	await user.click(
		await screen.findByRole("button", {
			name: "View logs for apply older-run",
		}),
	);
	await waitFor(() =>
		expect(API.getTemplateResourceRunLogs).toHaveBeenCalledWith(
			MockTemplate.id,
			mockHistorical.id,
		),
	);
});

it("retries an initial resource load failure", async () => {
	const user = userEvent.setup();
	const get = vi
		.spyOn(API, "getTemplateResources")
		.mockRejectedValueOnce(
			mockApiError({ message: "Resource state unavailable." }),
		)
		.mockResolvedValue(MockTemplateResources);
	render(<TemplateSharedResourcesPage />);
	await user.click(
		await screen.findByRole("button", { name: "Retry resources" }),
	);
	await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
});

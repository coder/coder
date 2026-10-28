import "#/testHelpers/monaco";
import { File as NodeFile } from "node:buffer";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { foundationKey } from "#/api/queries/foundations";
import type { MonacoEditorProps } from "#/pages/TemplateVersionEditorPage/MonacoEditor";
import { MockDefaultOrganization, mockApiError } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import {
	createMockTemplateResourceArchive,
	MockFoundation,
	MockFoundationPlanPreview,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";
import FoundationPage from "./FoundationPage";

vi.mock("#/modules/dashboard/Navbar/Navbar", () => ({
	Navbar: () => <nav aria-label="Main navigation" />,
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		experiments: ["template-resources"],
		organizations: [MockDefaultOrganization],
	}),
}));
vi.mock("#/pages/TemplateVersionEditorPage/MonacoEditor", () => ({
	MonacoEditor: (props: MonacoEditorProps) => (
		<textarea
			aria-label={props.path}
			value={props.value}
			onChange={(event) => props.onChange?.(event.target.value)}
		/>
	),
}));
beforeEach(async () => {
	vi.stubGlobal("File", NodeFile);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API, "getFoundation").mockResolvedValue({
		...MockFoundation,
		runs: [],
	});
	vi.spyOn(API, "getFile").mockResolvedValue(
		await createMockTemplateResourceArchive(),
	);
	vi.spyOn(API, "getFoundationRunLogs").mockResolvedValue([]);
	vi.spyOn(API, "getWorkspaces").mockResolvedValue({
		workspaces: [],
		count: 0,
	});
});
afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});
const renderPage = (
	search = "",
	organization = MockFoundation.organization_id,
) =>
	renderWithRouter(
		createMemoryRouter(
			[
				{ path: "/foundations", element: <div /> },
				{
					path: "/organizations/:organization/foundations/:foundation",
					element: <FoundationPage />,
				},
			],
			{
				initialEntries: [
					`/organizations/${organization}/foundations/${MockFoundation.id}${search}`,
				],
			},
		),
	);

it("edits a standalone multi-file Foundation without loading any workspace version or template", async () => {
	const template = vi.spyOn(API, "getTemplate");
	const version = vi.spyOn(API, "getTemplateVersion");
	const association = vi.spyOn(API, "getTemplateFoundation");
	const save = vi
		.spyOn(API, "createFoundationRevision")
		.mockResolvedValue(MockTemplateResourceRevision);
	vi.spyOn(API, "uploadFile").mockResolvedValue({ hash: "full-archive" });
	vi.spyOn(API, "createFoundationRun").mockResolvedValue({
		...MockTemplateResourceRun,
		action: "plan",
	});
	const user = userEvent.setup();
	const { queryClient } = renderPage("?edit=true");
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# draft",
	);
	await act(async () => {
		queryClient.setQueryData(
			foundationKey(MockFoundation.organization_id, MockFoundation.id),
			{ ...MockFoundation, name: "Updated server name", runs: [] },
		);
	});
	await user.click(screen.getByRole("button", { name: "Save files" }));
	await waitFor(() =>
		expect(save).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{ file_id: "full-archive" },
		),
	);
	expect(template).not.toHaveBeenCalled();
	expect(version).not.toHaveBeenCalled();
	expect(association).not.toHaveBeenCalled();
});

it("opens the independent editor from the Foundation menu", async () => {
	const user = userEvent.setup();
	const { router } = renderPage();
	await user.click(await screen.findByRole("button", { name: "Open menu" }));
	await user.click(await screen.findByRole("menuitem", { name: "Edit files" }));
	expect(router.state.location.search).toBe("?edit=true");
	await screen.findByRole("textbox", { name: "main.tf" });
	expect(API.getFile).toHaveBeenCalledWith(
		MockTemplateResourceRevision.file_id,
	);
});

it("routes each Foundation tab without enqueuing Terraform", async () => {
	const user = userEvent.setup();
	const run = vi.spyOn(API, "createFoundationRun");
	const { router } = renderPage();
	await user.click(await screen.findByRole("link", { name: "Resources" }));
	expect(router.state.location.search).toBe("?tab=resources");
	await user.click(screen.getByRole("link", { name: "Versions" }));
	expect(router.state.location.search).toBe("?tab=versions");
	await user.click(screen.getByRole("link", { name: "Runs" }));
	expect(router.state.location.search).toBe("?tab=runs");
	await user.click(screen.getByRole("link", { name: "Source Code" }));
	expect(router.state.location.search).toBe("?tab=files");
	await waitFor(() =>
		expect(API.getFile).toHaveBeenCalledWith(
			MockTemplateResourceRevision.file_id,
		),
	);
	expect(run).not.toHaveBeenCalled();
});

it("plans the current revision only after selecting Versions and Plan", async () => {
	const user = userEvent.setup();
	const run = vi
		.spyOn(API, "createFoundationRun")
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "plan" });
	renderPage();
	await user.click(await screen.findByRole("link", { name: "Versions" }));
	await user.click(await screen.findByRole("button", { name: "Plan" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{ revision_id: MockFoundation.current_revision_id, action: "plan" },
		),
	);
});

it.each([
	{ action: "apply", button: "Apply", confirm: "Apply Foundation" },
	{ action: "destroy", button: "Destroy", confirm: "Destroy shared resources" },
])(
	"confirms $action from Versions before changing infrastructure",
	async ({ action, button, confirm }) => {
		const user = userEvent.setup();
		const run = vi
			.spyOn(API, "createFoundationRun")
			.mockResolvedValue(MockTemplateResourceRun);
		renderPage("?tab=versions");
		await user.click(await screen.findByRole("button", { name: button }));
		expect(run).not.toHaveBeenCalled();
		await user.click(await screen.findByRole("button", { name: confirm }));
		await waitFor(() =>
			expect(run).toHaveBeenCalledWith(
				MockFoundation.organization_id,
				MockFoundation.id,
				{ revision_id: MockFoundation.current_revision_id, action },
			),
		);
	},
);

it.each([null, MockFoundation.applied_revision_id])(
	"destroys the failed Apply checkpoint independently of applied revision %s and retries",
	async (appliedRevisionId) => {
		const checkpointRevision = {
			...MockTemplateResourceRevision,
			id: "81000000-0000-4000-8000-000000000002",
		};
		vi.mocked(API.getFoundation).mockResolvedValue({
			...MockFoundation,
			status: "failed",
			applied_revision_id: appliedRevisionId,
			checkpoint_revision_id: checkpointRevision.id,
			revisions: [...MockFoundation.revisions, checkpointRevision],
		});
		const run = vi
			.spyOn(API, "createFoundationRun")
			.mockRejectedValueOnce(mockApiError({ message: "Cleanup unavailable." }))
			.mockResolvedValue({
				...MockTemplateResourceRun,
				action: "destroy",
				revision_id: checkpointRevision.id,
			});
		const user = userEvent.setup();
		const { queryClient } = renderPage("?tab=versions");
		await user.click(await screen.findByRole("button", { name: "Destroy" }));
		await user.click(
			await screen.findByRole("button", { name: "Destroy shared resources" }),
		);
		await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		await user.click(
			screen.getByRole("button", { name: "Destroy shared resources" }),
		);
		await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
		expect(run).toHaveBeenLastCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{ revision_id: checkpointRevision.id, action: "destroy" },
		);
	},
);

it.each([false, true])(
	"allows Apply with advisory preview available=%s without individual overrides",
	async (available) => {
		vi.mocked(API.getFoundation).mockResolvedValue({
			...MockFoundation,
			runs: [
				{
					...MockTemplateResourceRun,
					action: "plan",
					preview: { ...MockFoundationPlanPreview, available },
				},
			],
		});
		const run = vi
			.spyOn(API, "createFoundationRun")
			.mockResolvedValue(MockTemplateResourceRun);
		const user = userEvent.setup();
		renderPage("?tab=versions");
		await user.click(await screen.findByRole("button", { name: "Apply" }));
		await user.click(
			await screen.findByRole("button", { name: "Apply Foundation" }),
		);
		await waitFor(() =>
			expect(run).toHaveBeenCalledWith(
				MockFoundation.organization_id,
				MockFoundation.id,
				{ revision_id: MockFoundation.current_revision_id, action: "apply" },
			),
		);
	},
);

it("does not plan or apply without Foundation mutation permissions", async () => {
	vi.mocked(API.checkAuthorization).mockResolvedValue({
		canReadFoundation: true,
		canUpdateFoundation: false,
		canUseFoundation: false,
	});
	const run = vi.spyOn(API, "createFoundationRun");
	const user = userEvent.setup();
	renderPage("?tab=versions");
	await user.click(await screen.findByRole("button", { name: "Plan" }));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	await user.click(screen.getByRole("button", { name: "Destroy" }));
	expect(run).not.toHaveBeenCalled();
});

it("saves disabling workspace destruction protection without running Terraform", async () => {
	const update = vi.spyOn(API, "updateFoundation").mockResolvedValue({
		...MockFoundation,
		prevent_destroy_with_workspaces: false,
	});
	const run = vi.spyOn(API, "createFoundationRun");
	const user = userEvent.setup();
	renderPage("?tab=settings");
	await user.click(
		await screen.findByRole("checkbox", {
			name: "Prevent Destroy while workspaces exist",
		}),
	);
	await user.click(screen.getByRole("button", { name: "Save settings" }));
	await waitFor(() =>
		expect(update).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{
				name: MockFoundation.name,
				provisioner_tags: {},
				prevent_destroy_with_workspaces: false,
			},
		),
	);
	expect(run).not.toHaveBeenCalled();
	expect(API.getWorkspaces).not.toHaveBeenCalled();
});

it("deletes a cleaned Foundation separately even when consumer workspaces remain", async () => {
	vi.mocked(API.getFoundation).mockResolvedValue({
		...MockFoundation,
		status: "destroyed",
		applied_revision_id: null,
		consumers: { total: 2, running: 1, stopped: 1, unknown: 0 },
	});
	const remove = vi.spyOn(API, "deleteFoundation").mockResolvedValue(undefined);
	const run = vi.spyOn(API, "createFoundationRun");
	const user = userEvent.setup();
	const { router } = renderPage("?tab=settings");
	await user.click(
		await screen.findByRole("button", { name: "Delete Foundation" }),
	);
	expect(remove).not.toHaveBeenCalled();
	await user.click(
		await screen.findByRole("button", { name: "Confirm deletion" }),
	);
	await waitFor(() =>
		expect(remove).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
		),
	);
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/foundations"),
	);
	expect(run).not.toHaveBeenCalled();
});

it("does not delete a Foundation before cleanup", async () => {
	const remove = vi.spyOn(API, "deleteFoundation");
	const user = userEvent.setup();
	renderPage("?tab=settings");
	await user.click(
		await screen.findByRole("button", { name: "Delete Foundation" }),
	);
	expect(remove).not.toHaveBeenCalled();
});

it.each([MockDefaultOrganization.id, MockDefaultOrganization.name])(
	"saves settings and immediately refreshes the canonical query from organization route %s",
	async (organization) => {
		const user = userEvent.setup();
		const update = vi
			.spyOn(API, "updateFoundation")
			.mockResolvedValue(MockFoundation);
		const run = vi.spyOn(API, "createFoundationRun");
		const { queryClient, router } = renderPage("", organization);
		await user.click(await screen.findByRole("button", { name: "Open menu" }));
		await user.click(await screen.findByRole("menuitem", { name: "Settings" }));
		expect(router.state.location.search).toBe("?tab=settings");
		const name = await screen.findByRole("textbox", {
			name: "Foundation name",
		});
		await user.clear(name);
		await user.type(name, "renamed");
		await act(async () => {
			queryClient.setQueryData(
				foundationKey(MockFoundation.organization_id, MockFoundation.id),
				{ ...MockFoundation, name: "server-name" },
			);
		});
		await user.click(screen.getByRole("button", { name: "Save settings" }));
		await waitFor(() =>
			expect(update).toHaveBeenCalledWith(
				MockFoundation.organization_id,
				MockFoundation.id,
				{
					name: "renamed",
					provisioner_tags: {},
					prevent_destroy_with_workspaces: true,
				},
			),
		);
		await waitFor(() => expect(API.getFoundation).toHaveBeenCalledTimes(2));
		expect(API.getFoundation).toHaveBeenLastCalledWith(
			MockDefaultOrganization.id,
			MockFoundation.id,
		);
		expect(
			queryClient.getQueryData(
				foundationKey(MockDefaultOrganization.name, MockFoundation.id),
			),
		).toBeUndefined();
		expect(run).not.toHaveBeenCalled();
	},
);

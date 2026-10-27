import "#/testHelpers/monaco";
import { File as NodeFile } from "node:buffer";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { templateResourcesKey } from "#/api/queries/templateResources";
import type { TemplateResources } from "#/api/typesGenerated";
import type { DashboardValue } from "#/modules/dashboard/DashboardProvider";
import {
	MockTemplate,
	MockTemplateVersion,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	createMockTemplateResourceArchive,
	MockTemplateResourceFiles,
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
	MockTemplateResources,
} from "#/testHelpers/templateResources";
import { getFileText } from "#/utils/filetree";
import { TarReader } from "#/utils/tar";
import type { MonacoEditorProps } from "./MonacoEditor";
import TemplateFilesEditorPage from "./TemplateFilesEditorPage";

const dashboard = vi.hoisted(
	(): Pick<DashboardValue, "experiments" | "showOrganizations"> => ({
		experiments: ["template-resources"],
		showOrganizations: true,
	}),
);
vi.mock(import("#/modules/dashboard/useDashboard"), async (importOriginal) => {
	const original = await importOriginal();
	return {
		...original,
		useDashboard: () => ({ ...original.useDashboard(), ...dashboard }),
	};
});
vi.mock("./MonacoEditor", () => ({
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
	const archive = await createMockTemplateResourceArchive();
	vi.spyOn(API, "getFile").mockResolvedValue(archive);
	vi.spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
		MockTemplateResourceLog,
	]);
	dashboard.experiments = ["template-resources"];
	vi.spyOn(API, "getTemplateResources").mockResolvedValue({
		...MockTemplateResources,
		runs: [],
	});
});

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

const editorPath = `/templates/${MockTemplate.organization_name}/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`;
const renderEditor = (search = "?path=main.tf") =>
	renderWithAuth(<TemplateFilesEditorPage />, {
		route: `${editorPath}${search}`,
		path: "/templates/:organization/:template/versions/:version/edit",
		extraRoutes: [
			{ path: "/templates/:organization/:template", element: <div /> },
		],
	});

it("builds and publishes the shared resource archive independently of per-workspace versions", async () => {
	const user = userEvent.setup();
	const publishWorkspace = vi.spyOn(API, "updateActiveTemplateVersion");
	const buildWorkspace = vi.spyOn(API, "createTemplateVersion");
	const mockSavedRevision = {
		...MockTemplateResourceRevision,
		id: "new-revision",
	};
	let mockResources: TemplateResources = { ...MockTemplateResources, runs: [] };
	vi.mocked(API.getTemplateResources).mockImplementation(
		async () => mockResources,
	);
	const upload = vi
		.spyOn(API, "uploadFile")
		.mockResolvedValue({ hash: "uploaded-archive" });
	const save = vi
		.spyOn(API, "createTemplateResourceRevision")
		.mockImplementation(async () => {
			mockResources = {
				...mockResources,
				current_revision_id: mockSavedRevision.id,
				revisions: [mockSavedRevision],
			};
			return mockSavedRevision;
		});
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockImplementation(async (_id, request) => {
			const result = {
				...MockTemplateResourceRun,
				...request,
				id: request.action,
				job: { ...MockTemplateResourceRun.job, status: "succeeded" as const },
			};
			mockResources = {
				...mockResources,
				runs: [result, ...mockResources.runs],
				applied_revision_id:
					request.action === "apply"
						? request.revision_id
						: mockResources.applied_revision_id,
			};
			return result;
		});
	const { router, queryClient } = renderEditor();
	await screen.findByRole("textbox", { name: "main.tf" });
	await user.click(
		within(
			screen.getByRole("navigation", { name: "Provisioning scopes" }),
		).getByRole("link", { name: "Shared" }),
	);
	await screen.findByRole("region", { name: "Build output" });
	expect(router.state.location.pathname).toBe(editorPath);
	expect(router.state.location.search).toBe("?path=main.tf&scope=template");
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# edited",
	);
	await user.click(
		screen.getByRole("button", { name: "Increase editor height" }),
	);
	await user.click(screen.getByRole("button", { name: "Publish" }));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Build" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(MockTemplate.id, {
			revision_id: mockSavedRevision.id,
			action: "plan",
		}),
	);
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(save).toHaveBeenCalledWith(MockTemplate.id, {
		file_id: "uploaded-archive",
	});
	const uploaded = upload.mock.calls[0]?.[0];
	if (!uploaded) throw new Error("Build did not upload an archive");
	const archive = new TarReader();
	await archive.readFile(uploaded);
	expect(archive.getTextFile("main.tf")).toBe(
		`${getFileText("main.tf", MockTemplateResourceFiles)}\n# edited`,
	);
	expect(archive.getTextFile("modules/coder-network-policy/variables.tf")).toBe(
		getFileText(
			"modules/coder-network-policy/variables.tf",
			MockTemplateResourceFiles,
		),
	);
	await user.click(screen.getByRole("button", { name: "Publish" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(MockTemplate.id, {
			revision_id: mockSavedRevision.id,
			action: "apply",
		}),
	);
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await user.click(screen.getByRole("button", { name: "Publish" }));
	expect(run).toHaveBeenCalledTimes(2);
	expect(publishWorkspace).not.toHaveBeenCalled();
	expect(buildWorkspace).not.toHaveBeenCalled();
	await user.click(screen.getByRole("link", { name: "Per-workspace" }));
	expect(router.state.location.pathname).toBe(editorPath);
	expect(router.state.location.search).toBe("?path=main.tf");
});

it("opens shared scope directly without fetching a workspace version and returns to the template", async () => {
	const user = userEvent.setup();
	const version = vi.spyOn(API, "getTemplateVersionByName");
	const { router } = renderEditor("?scope=template");
	await screen.findByRole("button", { name: "Publish" });
	expect(version).not.toHaveBeenCalled();
	await user.click(screen.getByRole("link", { name: "Back to the template" }));
	expect(router.state.location.pathname).toBe(
		`/templates/${MockTemplate.organization_name}/${MockTemplate.name}`,
	);
});

it("can switch to shared resources while the workspace version is unavailable", async () => {
	vi.spyOn(API, "getTemplateVersionByName").mockRejectedValue(
		mockApiError({ message: "Version unavailable." }),
	);
	const user = userEvent.setup();
	const { router } = renderEditor();
	await user.click(await screen.findByRole("link", { name: "Shared" }));
	await screen.findByRole("textbox", { name: "main.tf" });
	expect(router.state.location.search).toBe("?path=main.tf&scope=template");
	expect(API.getTemplateResources).toHaveBeenCalledWith(MockTemplate.id);
});

it("retries a failed template lookup in shared scope", async () => {
	const get = vi
		.spyOn(API, "getTemplateByName")
		.mockRejectedValueOnce(mockApiError({ message: "Template unavailable." }))
		.mockResolvedValue(MockTemplate);
	const user = userEvent.setup();
	renderEditor("?scope=template");
	await user.click(await screen.findByRole("button", { name: "Retry editor" }));
	await screen.findByRole("textbox", { name: "main.tf" });
	expect(get).toHaveBeenCalledTimes(2);
	expect(API.getTemplateResources).toHaveBeenCalledWith(MockTemplate.id);
});

it("preserves the file selection in per-workspace scope when the experiment is disabled", async () => {
	dashboard.experiments = [];
	const { router } = renderEditor("?path=main.tf&scope=template");
	await screen.findByRole("button", { name: "Build" });
	expect(router.state.location.pathname).toBe(editorPath);
	expect(router.state.location.search).toBe("?path=main.tf");
	expect(API.getTemplateResources).not.toHaveBeenCalled();
});

it("does not fetch shared resources without update permission", async () => {
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		canUpdateTemplate: false,
	});
	const user = userEvent.setup();
	const { router } = renderEditor("?scope=template");
	await screen.findByRole("alert");
	await user.click(
		await screen.findByRole("link", { name: "Back to the template" }),
	);
	expect(router.state.location.pathname).toBe(
		`/templates/${MockTemplate.organization_name}/${MockTemplate.name}`,
	);
	expect(API.getTemplateResources).not.toHaveBeenCalled();
});

it("retains all draft files after a failed build and background refresh, then retries", async () => {
	const user = userEvent.setup();
	const upload = vi
		.spyOn(API, "uploadFile")
		.mockResolvedValue({ hash: "archive" });
	const save = vi
		.spyOn(API, "createTemplateResourceRevision")
		.mockRejectedValueOnce(mockApiError({ message: "Save failed." }))
		.mockResolvedValue(MockTemplateResourceRevision);
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "plan" });
	const { queryClient } = renderEditor("?scope=template");
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# retained draft",
	);
	await user.click(screen.getByRole("button", { name: "Build" }));
	await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await act(async () => {
		queryClient.setQueryData(templateResourcesKey(MockTemplate.id), {
			...MockTemplateResources,
			current_revision_id: "external",
			revisions: [
				{
					...MockTemplateResourceRevision,
					id: "external",
					source: "external source",
				},
			],
		});
	});
	await user.click(screen.getByRole("button", { name: "Publish" }));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Build" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(MockTemplate.id, {
			revision_id: MockTemplateResourceRevision.id,
			action: "plan",
		}),
	);
	for (const [file] of upload.mock.calls) {
		const archive = new TarReader();
		await archive.readFile(file);
		expect(archive.getTextFile("main.tf")).toBe(
			`${getFileText("main.tf", MockTemplateResourceFiles)}\n# retained draft`,
		);
		expect(archive.getTextFile("modules/coder-network-policy/main.tf")).toBe(
			getFileText(
				"modules/coder-network-policy/main.tf",
				MockTemplateResourceFiles,
			),
		);
	}
});

it("retains the saved archive when plan enqueue fails and refreshes revision history", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "uploadFile").mockResolvedValue({ hash: "saved-archive" });
	const save = vi
		.spyOn(API, "createTemplateResourceRevision")
		.mockResolvedValue(MockTemplateResourceRevision);
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockRejectedValueOnce(mockApiError({ message: "Plan unavailable." }))
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "plan" });
	const { queryClient } = renderEditor("?scope=template");
	const invalidate = vi.spyOn(queryClient, "invalidateQueries");
	await user.click(await screen.findByRole("button", { name: "Build" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(save).toHaveBeenCalledWith(MockTemplate.id, {
		file_id: "saved-archive",
	});
	expect(invalidate).toHaveBeenCalledWith({
		queryKey: templateResourcesKey(MockTemplate.id),
		exact: true,
	});
	await user.click(screen.getByRole("button", { name: "Publish" }));
	expect(run).toHaveBeenCalledTimes(1);
	await user.click(screen.getByRole("button", { name: "Build" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
	expect(run).toHaveBeenLastCalledWith(MockTemplate.id, {
		revision_id: MockTemplateResourceRevision.id,
		action: "plan",
	});
});

it.each(["pending", "failed", "canceled"] as const)(
	"does not publish a %s plan",
	async (status) => {
		const user = userEvent.setup();
		vi.mocked(API.getTemplateResources).mockResolvedValue({
			...MockTemplateResources,
			applied_revision_id: null,
			runs: [
				{
					...MockTemplateResourceRun,
					action: "plan",
					job: { ...MockTemplateResourceRun.job, status },
				},
			],
		});
		const run = vi.spyOn(API, "createTemplateResourceRun");
		renderEditor("?scope=template");
		await user.click(await screen.findByRole("button", { name: "Publish" }));
		expect(run).not.toHaveBeenCalled();
	},
);

it("invalidates a successful build when files change, including during a running plan", async () => {
	const user = userEvent.setup();
	const mockPlanned = {
		...MockTemplateResourceRun,
		action: "plan",
		job: { ...MockTemplateResourceRun.job, status: "running" as const },
	};
	vi.mocked(API.getTemplateResources).mockResolvedValue({
		...MockTemplateResources,
		applied_revision_id: null,
		runs: [mockPlanned],
	});
	const run = vi.spyOn(API, "createTemplateResourceRun");
	const { queryClient } = renderEditor("?scope=template");
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# new draft",
	);
	await act(async () => {
		queryClient.setQueryData(templateResourcesKey(MockTemplate.id), {
			...MockTemplateResources,
			applied_revision_id: null,
			runs: [
				{ ...mockPlanned, job: { ...mockPlanned.job, status: "succeeded" } },
			],
		});
	});
	await user.click(screen.getByRole("button", { name: "Publish" }));
	expect(run).not.toHaveBeenCalled();
});

it("retries publishing a successful saved plan without creating a revision", async () => {
	const user = userEvent.setup();
	vi.mocked(API.getTemplateResources).mockResolvedValue({
		...MockTemplateResources,
		applied_revision_id: null,
		runs: [{ ...MockTemplateResourceRun, action: "plan" }],
	});
	const save = vi.spyOn(API, "createTemplateResourceRevision");
	const run = vi
		.spyOn(API, "createTemplateResourceRun")
		.mockRejectedValueOnce(
			mockApiError({ message: "Provisioner unavailable." }),
		)
		.mockResolvedValue(MockTemplateResourceRun);
	const { queryClient } = renderEditor("?scope=template");
	await user.click(await screen.findByRole("button", { name: "Publish" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await user.click(screen.getByRole("button", { name: "Publish" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
	expect(run).toHaveBeenLastCalledWith(MockTemplate.id, {
		revision_id: MockTemplateResourceRevision.id,
		action: "apply",
	});
	expect(save).not.toHaveBeenCalled();
});

it("retries downloading the full archive rather than falling back to source", async () => {
	const user = userEvent.setup();
	vi.mocked(API.getFile).mockRejectedValueOnce(
		mockApiError({ message: "Files unavailable." }),
	);
	renderEditor("?scope=template");
	await user.click(await screen.findByRole("button", { name: "Retry files" }));
	await screen.findByRole("textbox", { name: "main.tf" });
	expect(API.getFile).toHaveBeenNthCalledWith(
		2,
		MockTemplateResourceRevision.file_id,
	);
});

it.each(["per-workspace", "shared"])(
	"protects unsaved %s changes when switching provisioning scopes",
	async (scope) => {
		const user = userEvent.setup();
		const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
		const sharedScope = scope === "shared";
		const search = sharedScope
			? "?path=main.tf&scope=template"
			: "?path=main.tf";
		const { router } = renderEditor(search);
		await user.type(
			await screen.findByRole("textbox", { name: "main.tf" }),
			"draft",
		);
		await user.click(
			screen.getByRole("link", {
				name: sharedScope ? "Per-workspace" : "Shared",
			}),
		);
		await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
		expect(router.state.location.search).toBe(search);
		const unload = new Event("beforeunload", { cancelable: true });
		window.dispatchEvent(unload);
		expect(unload.defaultPrevented).toBe(true);
	},
);

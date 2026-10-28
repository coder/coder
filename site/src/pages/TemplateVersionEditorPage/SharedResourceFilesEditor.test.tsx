import "#/testHelpers/monaco";
import { File as NodeFile } from "node:buffer";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { foundationKey, foundationsKey } from "#/api/queries/foundations";
import type { Foundation } from "#/api/typesGenerated";
import type { DashboardValue } from "#/modules/dashboard/DashboardProvider";
import FoundationPage from "#/pages/FoundationsPage/FoundationPage";
import { mockApiError } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	createMockTemplateResourceArchive,
	MockFoundation,
	MockTemplateResourceFiles,
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";
import { getFileText } from "#/utils/filetree";
import { TarReader } from "#/utils/tar";
import type { MonacoEditorProps } from "./MonacoEditor";

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
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API, "getWorkspaces").mockResolvedValue({
		workspaces: [],
		count: 0,
	});
	vi.stubGlobal("File", NodeFile);
	const archive = await createMockTemplateResourceArchive();
	vi.spyOn(API, "getFile").mockResolvedValue(archive);
	vi.spyOn(API, "getFoundationRunLogs").mockResolvedValue([
		MockTemplateResourceLog,
	]);
	dashboard.experiments = ["template-resources"];
	vi.spyOn(API, "getFoundation").mockResolvedValue({
		...MockFoundation,
		runs: [],
	});
});

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

const editorPath = `/organizations/${MockFoundation.organization_id}/foundations/${MockFoundation.id}`;
const renderEditor = () =>
	renderWithAuth(<FoundationPage />, {
		route: `${editorPath}?edit=true&path=main.tf`,
		path: "/organizations/:organization/foundations/:foundation",
	});

it("saves, plans, applies, and reapplies the Foundation archive independently of workspace versions", async () => {
	const user = userEvent.setup();
	const publishWorkspace = vi.spyOn(API, "updateActiveTemplateVersion");
	const buildWorkspace = vi.spyOn(API, "createTemplateVersion");
	const mockSavedRevision = {
		...MockTemplateResourceRevision,
		id: "new-revision",
	};
	let mockResources: Foundation = { ...MockFoundation, runs: [] };
	vi.mocked(API.getFoundation).mockImplementation(async () => mockResources);
	const upload = vi
		.spyOn(API, "uploadFile")
		.mockResolvedValue({ hash: "uploaded-archive" });
	const save = vi
		.spyOn(API, "createFoundationRevision")
		.mockImplementation(async () => {
			mockResources = {
				...mockResources,
				current_revision_id: mockSavedRevision.id,
				revisions: [mockSavedRevision],
			};
			return mockSavedRevision;
		});
	const run = vi
		.spyOn(API, "createFoundationRun")
		.mockImplementation(async (_organization, _id, request) => {
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
	const { queryClient } = renderEditor();
	await screen.findByRole("region", { name: "Terraform output" });
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# edited",
	);
	await user.click(
		screen.getByRole("button", { name: "Increase editor height" }),
	);
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Save files" }));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Plan" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{
				revision_id: mockSavedRevision.id,
				action: "plan",
			},
		),
	);
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(save).toHaveBeenCalledWith(
		MockFoundation.organization_id,
		MockFoundation.id,
		{
			file_id: "uploaded-archive",
		},
	);
	const uploaded = upload.mock.calls[0]?.[0];
	if (!uploaded) throw new Error("Save did not upload an archive");
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
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(run).toHaveBeenCalledTimes(1);
	await user.click(screen.getByRole("button", { name: "Apply Foundation" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{
				revision_id: mockSavedRevision.id,
				action: "apply",
			},
		),
	);
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await user.click(screen.getByRole("button", { name: "Apply" }));
	await user.click(screen.getByRole("button", { name: "Apply Foundation" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(3));
	expect(publishWorkspace).not.toHaveBeenCalled();
	expect(buildWorkspace).not.toHaveBeenCalled();
});

it("opens the standalone editor without fetching a workspace version", async () => {
	const version = vi.spyOn(API, "getTemplateVersionByName");
	renderEditor();
	await screen.findByRole("button", { name: "Apply" });
	expect(version).not.toHaveBeenCalled();
});

it("allows Foundation editing without template update permission", async () => {
	vi.mocked(API.checkAuthorization).mockImplementation(async ({ checks }) =>
		Object.fromEntries(
			Object.keys(checks).map((key) => [key, key !== "canUpdateTemplate"]),
		),
	);
	const save = vi
		.spyOn(API, "createFoundationRevision")
		.mockResolvedValue(MockTemplateResourceRevision);
	vi.spyOn(API, "uploadFile").mockResolvedValue({ hash: "archive" });
	vi.spyOn(API, "createFoundationRun").mockResolvedValue(
		MockTemplateResourceRun,
	);
	const user = userEvent.setup();
	renderEditor();
	await user.click(await screen.findByRole("button", { name: "Save files" }));
	await waitFor(() =>
		expect(save).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{ file_id: "archive" },
		),
	);
});

it("retains all draft files after a failed save and background refresh, then retries", async () => {
	const user = userEvent.setup();
	const upload = vi
		.spyOn(API, "uploadFile")
		.mockResolvedValue({ hash: "archive" });
	const save = vi
		.spyOn(API, "createFoundationRevision")
		.mockRejectedValueOnce(mockApiError({ message: "Save failed." }))
		.mockResolvedValue(MockTemplateResourceRevision);
	const run = vi
		.spyOn(API, "createFoundationRun")
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "plan" });
	const { queryClient } = renderEditor();
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# retained draft",
	);
	await user.click(screen.getByRole("button", { name: "Save files" }));
	await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	await act(async () => {
		queryClient.setQueryData(
			foundationKey(MockFoundation.organization_id, MockFoundation.id),
			{
				...MockFoundation,
				current_revision_id: "external",
				revisions: [
					{
						...MockTemplateResourceRevision,
						id: "external",
						source: "external source",
					},
				],
			},
		);
	});
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Save files" }));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Plan" }));
	await waitFor(() =>
		expect(run).toHaveBeenCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{
				revision_id: MockTemplateResourceRevision.id,
				action: "plan",
			},
		),
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
		.spyOn(API, "createFoundationRevision")
		.mockResolvedValue(MockTemplateResourceRevision);
	const run = vi
		.spyOn(API, "createFoundationRun")
		.mockRejectedValueOnce(mockApiError({ message: "Plan unavailable." }))
		.mockResolvedValue({ ...MockTemplateResourceRun, action: "plan" });
	const { queryClient } = renderEditor();
	const invalidate = vi.spyOn(queryClient, "invalidateQueries");
	await user.click(await screen.findByRole("button", { name: "Save files" }));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(run).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Plan" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	expect(save).toHaveBeenCalledWith(
		MockFoundation.organization_id,
		MockFoundation.id,
		{
			file_id: "saved-archive",
		},
	);
	expect(invalidate).toHaveBeenCalledWith({
		queryKey: foundationsKey(MockFoundation.organization_id),
		exact: true,
	});
	await user.click(screen.getByRole("button", { name: "Plan" }));
	await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
	expect(save).toHaveBeenCalledTimes(1);
	expect(run).toHaveBeenLastCalledWith(
		MockFoundation.organization_id,
		MockFoundation.id,
		{
			revision_id: MockTemplateResourceRevision.id,
			action: "plan",
		},
	);
});

it.each(["pending", "running", "canceling"] as const)(
	"does not apply during a %s plan",
	async (status) => {
		const user = userEvent.setup();
		vi.mocked(API.getFoundation).mockResolvedValue({
			...MockFoundation,
			applied_revision_id: null,
			runs: [
				{
					...MockTemplateResourceRun,
					action: "plan",
					job: { ...MockTemplateResourceRun.job, status },
				},
			],
		});
		const run = vi.spyOn(API, "createFoundationRun");
		renderEditor();
		await user.click(await screen.findByRole("button", { name: "Apply" }));
		expect(run).not.toHaveBeenCalled();
	},
);

it("requires saving edited files before applying, including edits during a running plan", async () => {
	const user = userEvent.setup();
	const mockPlanned = {
		...MockTemplateResourceRun,
		action: "plan",
		job: { ...MockTemplateResourceRun.job, status: "running" as const },
	};
	vi.mocked(API.getFoundation).mockResolvedValue({
		...MockFoundation,
		applied_revision_id: null,
		runs: [mockPlanned],
	});
	const run = vi.spyOn(API, "createFoundationRun");
	const { queryClient } = renderEditor();
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"\n# new draft",
	);
	await act(async () => {
		queryClient.setQueryData(
			foundationKey(MockFoundation.organization_id, MockFoundation.id),
			{
				...MockFoundation,
				applied_revision_id: null,
				runs: [
					{ ...mockPlanned, job: { ...mockPlanned.job, status: "succeeded" } },
				],
			},
		);
	});
	await user.click(screen.getByRole("button", { name: "Apply" }));
	expect(run).not.toHaveBeenCalled();
});

it.each(["failed", "canceled", "succeeded"] as const)(
	"applies after a %s preview and retries without creating a revision",
	async (status) => {
		const user = userEvent.setup();
		vi.mocked(API.getFoundation).mockResolvedValue({
			...MockFoundation,
			applied_revision_id: null,
			runs: [
				{
					...MockTemplateResourceRun,
					action: "plan",
					job: { ...MockTemplateResourceRun.job, status },
				},
			],
		});
		const save = vi.spyOn(API, "createFoundationRevision");
		const run = vi
			.spyOn(API, "createFoundationRun")
			.mockRejectedValueOnce(
				mockApiError({ message: "Provisioner unavailable." }),
			)
			.mockResolvedValue(MockTemplateResourceRun);
		const { queryClient } = renderEditor();
		await user.click(await screen.findByRole("button", { name: "Apply" }));
		expect(run).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Apply Foundation" }));
		await waitFor(() => expect(run).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		await user.click(screen.getByRole("button", { name: "Apply Foundation" }));
		await waitFor(() => expect(run).toHaveBeenCalledTimes(2));
		expect(run).toHaveBeenLastCalledWith(
			MockFoundation.organization_id,
			MockFoundation.id,
			{
				revision_id: MockTemplateResourceRevision.id,
				action: "apply",
			},
		);
		expect(save).not.toHaveBeenCalled();
	},
);

it("retries downloading the full archive rather than falling back to source", async () => {
	const user = userEvent.setup();
	vi.mocked(API.getFile).mockRejectedValueOnce(
		mockApiError({ message: "Files unavailable." }),
	);
	renderEditor();
	await user.click(await screen.findByRole("button", { name: "Retry files" }));
	await screen.findByRole("textbox", { name: "main.tf" });
	expect(API.getFile).toHaveBeenNthCalledWith(
		2,
		MockTemplateResourceRevision.file_id,
	);
});

it.each(["pending", "unknown"])(
	"saves edits independently while Foundation state is %s",
	async (status) => {
		vi.mocked(API.getFoundation).mockResolvedValue({
			...MockFoundation,
			status,
			runs:
				status === "pending"
					? [
							{
								...MockTemplateResourceRun,
								job: { ...MockTemplateResourceRun.job, status: "running" },
							},
						]
					: [],
		});
		const save = vi
			.spyOn(API, "createFoundationRevision")
			.mockResolvedValue(MockTemplateResourceRevision);
		vi.spyOn(API, "uploadFile").mockResolvedValue({
			hash: "saved-while-blocked",
		});
		const run = vi.spyOn(API, "createFoundationRun");
		const user = userEvent.setup();
		renderEditor();
		await user.type(
			await screen.findByRole("textbox", { name: "main.tf" }),
			"\n# saved draft",
		);
		await user.keyboard("{Control>}s{/Control}");
		await waitFor(() =>
			expect(save).toHaveBeenCalledWith(
				MockFoundation.organization_id,
				MockFoundation.id,
				{ file_id: "saved-while-blocked" },
			),
		);
		await user.click(screen.getByRole("button", { name: "Plan" }));
		await user.click(screen.getByRole("button", { name: "Apply" }));
		expect(run).not.toHaveBeenCalled();
	},
);

it("protects unsaved changes when leaving the Foundation editor", async () => {
	const user = userEvent.setup();
	const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
	const { router } = renderEditor();
	await user.type(
		await screen.findByRole("textbox", { name: "main.tf" }),
		"draft",
	);
	await user.click(screen.getByRole("link", { name: "Back to Foundation" }));
	await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
	expect(router.state.location.search).toBe("?edit=true&path=main.tf");
	const unload = new Event("beforeunload", { cancelable: true });
	window.dispatchEvent(unload);
	expect(unload.defaultPrevented).toBe(true);
});

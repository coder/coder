import "#/testHelpers/monaco";
import { screen, waitFor } from "@testing-library/react";
import { API } from "#/api/api";
import type { DashboardValue } from "#/modules/dashboard/DashboardProvider";
import { MockTemplate, MockTemplateVersion } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { createMockTemplateResourceArchive } from "#/testHelpers/templateResources";
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
	MonacoEditor: () => <textarea aria-label="main.tf" />,
}));
beforeEach(async () => {
	dashboard.experiments = ["template-resources"];
	vi.spyOn(API, "getFile").mockResolvedValue(
		await createMockTemplateResourceArchive(),
	);
});
afterEach(() => vi.restoreAllMocks());

const editorPath = `/templates/${MockTemplate.organization_name}/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`;
const associationPath = `/templates/${MockTemplate.organization_name}/${MockTemplate.name}/resources`;
const renderEditor = (search: string) =>
	renderWithAuth(<TemplateFilesEditorPage />, {
		route: `${editorPath}${search}`,
		path: "/templates/:organization/:template/versions/:version/edit",
		extraRoutes: [{ path: associationPath, element: <div /> }],
	});

it("redirects legacy Foundation editing links to the association page without loading a workspace version", async () => {
	const version = vi.spyOn(API, "getTemplateVersionByName");
	const foundation = vi.spyOn(API, "getTemplateFoundation");
	const { router } = renderEditor("?scope=template&path=main.tf");
	await waitFor(() =>
		expect(router.state.location.pathname).toBe(associationPath),
	);
	expect(router.state.location.search).toBe("");
	expect(version).not.toHaveBeenCalled();
	expect(foundation).not.toHaveBeenCalled();
});

it.each([true, false])(
	"keeps workspace file editing independent of Foundations with the experiment %s",
	async (enabled) => {
		dashboard.experiments = enabled ? ["template-resources"] : [];
		const foundation = vi.spyOn(API, "getTemplateFoundation");
		const { router } = renderEditor(
			enabled ? "?path=main.tf" : "?path=main.tf&scope=template",
		);
		await screen.findByRole("button", { name: "Build" });
		expect(router.state.location.pathname).toBe(editorPath);
		expect(router.state.location.search).toBe("?path=main.tf");
		expect(foundation).not.toHaveBeenCalled();
	},
);

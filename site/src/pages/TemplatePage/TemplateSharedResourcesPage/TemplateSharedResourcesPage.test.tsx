import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { foundationPath } from "#/api/queries/foundations";
import {
	MockTemplate,
	MockTemplateVersion,
	mockApiError,
} from "#/testHelpers/entities";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
import { MockFoundation } from "#/testHelpers/templateResources";
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
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API, "getTemplateFoundation").mockResolvedValue(MockFoundation);
});
afterEach(() => vi.restoreAllMocks());

it("navigates to the associated Foundation without loading its lifecycle data", async () => {
	const user = userEvent.setup();
	const logs = vi.spyOn(API, "getFoundationRunLogs");
	const run = vi.spyOn(API, "createFoundationRun");
	const version = vi.spyOn(API, "getTemplateVersionByName");
	const path = foundationPath(
		MockFoundation.organization_id,
		MockFoundation.id,
	);
	const { router } = renderWithRouter(
		createMemoryRouter([
			{ path: "/", element: <TemplateSharedResourcesPage /> },
			{ path, element: <div /> },
		]),
	);
	await user.click(
		await screen.findByRole("link", { name: MockFoundation.name }),
	);
	expect(router.state.location.pathname).toBe(path);
	expect(logs).not.toHaveBeenCalled();
	expect(run).not.toHaveBeenCalled();
	expect(version).not.toHaveBeenCalled();
});

it("retries an initial association load failure", async () => {
	const user = userEvent.setup();
	vi.mocked(API.getTemplateFoundation).mockRejectedValueOnce(
		mockApiError({ message: "Association unavailable." }),
	);
	render(<TemplateSharedResourcesPage />);
	await user.click(
		await screen.findByRole("button", { name: "Retry Foundation" }),
	);
	await waitFor(() =>
		expect(API.getTemplateFoundation).toHaveBeenCalledTimes(2),
	);
});

it("retries permissions before loading the association", async () => {
	const user = userEvent.setup();
	vi.mocked(API.checkAuthorization).mockRejectedValueOnce(
		mockApiError({ message: "Permissions unavailable." }),
	);
	render(<TemplateSharedResourcesPage />);
	await user.click(
		await screen.findByRole("button", { name: "Retry Foundation" }),
	);
	await waitFor(() =>
		expect(API.getTemplateFoundation).toHaveBeenCalledWith(MockTemplate.id),
	);
});

it("does not load the association without Foundation read permission", async () => {
	vi.mocked(API.checkAuthorization).mockResolvedValue({
		canReadFoundation: false,
	});
	render(<TemplateSharedResourcesPage />);
	await screen.findByRole("alert");
	expect(API.getTemplateFoundation).not.toHaveBeenCalled();
});

it("attaches a selected existing Foundation to an unbound template", async () => {
	vi.mocked(API.getTemplateFoundation).mockResolvedValue(null);
	vi.spyOn(API, "getFoundations").mockResolvedValue([
		{ ...MockFoundation, template_id: null },
	]);
	const bind = vi
		.spyOn(API, "setTemplateFoundation")
		.mockResolvedValue(MockFoundation);
	const user = userEvent.setup();
	render(<TemplateSharedResourcesPage />);
	await user.click(
		await screen.findByRole("combobox", {
			name: "Available organization Foundations",
		}),
	);
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(MockFoundation.name),
		}),
	);
	await user.click(screen.getByRole("button", { name: "Attach Foundation" }));
	expect(bind).not.toHaveBeenCalled();
	await user.click(
		await screen.findByRole("button", { name: "Confirm attachment" }),
	);
	await waitFor(() =>
		expect(bind).toHaveBeenCalledWith(MockTemplate.id, {
			foundation_id: MockFoundation.id,
		}),
	);
});

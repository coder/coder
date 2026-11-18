import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { API } from "#/api/api";
import type { AuthorizationRequest } from "#/api/typesGenerated";
import CreateTemplateGalleryPage from "#/pages/CreateTemplateGalleryPage/CreateTemplateGalleryPage";
import CreateTemplatePage from "#/pages/CreateTemplatePage/CreateTemplatePage";
import TemplateBuilderPage from "#/pages/TemplateBuilder/TemplateBuilderPage";
import TemplateSettingsPage from "#/pages/TemplateSettingsPage/TemplateGeneralSettingsPage/TemplateSettingsPage";
import TemplatePermissionsPage from "#/pages/TemplateSettingsPage/TemplatePermissionsPage/TemplatePermissionsPage";
import TemplateSchedulePage from "#/pages/TemplateSettingsPage/TemplateSchedulePage/TemplateSchedulePage";
import TemplateVariablesPage from "#/pages/TemplateSettingsPage/TemplateVariablesPage/TemplateVariablesPage";
import TemplateVersionEditorPage from "#/pages/TemplateVersionEditorPage/TemplateVersionEditorPage";
import TemplateVersionPage from "#/pages/TemplateVersionPage/TemplateVersionPage";
import {
	MockEntitlements,
	MockTemplate,
	MockTemplateVersion,
	mockApiError,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	renderWithTemplateSettingsLayout,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import TemplateFilesPage from "./TemplateFilesPage/TemplateFilesPage";
import { TemplateLayout } from "./TemplateLayout";
import { TemplatePageHeader } from "./TemplatePageHeader";
import TemplatePrebuildsPage from "./TemplatePrebuildsPage/TemplatePrebuildsPage";

const denyTemplateMutations = () => {
	server.use(
		http.post<never, AuthorizationRequest>(
			"/api/v2/authcheck",
			async ({ request }) => {
				const { checks } = await request.json();
				return HttpResponse.json(
					Object.fromEntries(
						Object.entries(checks).map(([name, check]) => [
							name,
							check.object.resource_type !== "template" ||
								!["create", "update", "delete"].includes(check.action),
						]),
					),
				);
			},
		),
	);
};

const deniedPages = [
	{ route: "/templates/new", element: <CreateTemplatePage /> },
	{
		route: "/templates/new?exampleId=aws-windows",
		element: <CreateTemplatePage />,
	},
	{
		route: `/templates/new?fromTemplate=${MockTemplate.id}`,
		element: <CreateTemplatePage />,
	},
	{ route: "/starter-templates", element: <CreateTemplateGalleryPage /> },
	{
		route: "/templates/new/builder?base=quickstart",
		element: <TemplateBuilderPage />,
	},
	{
		route: `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`,
		path: "/templates/:template/versions/:version/edit",
		element: <TemplateVersionEditorPage />,
	},
	...[
		{ path: "files", element: <TemplateFilesPage /> },
		{ path: "prebuilds", element: <TemplatePrebuildsPage /> },
	].map(({ path, element }) => ({
		route: `/templates/${MockTemplate.name}/${path}`,
		path: "/templates/:template",
		element: <TemplateLayout />,
		children: [{ path, element }],
	})),
];

test.each(deniedPages)(
	"denies $route before loading restricted page data",
	async ({ route, element, ...options }) => {
		denyTemplateMutations();
		const file = vi.spyOn(API, "getFile");
		const variables = vi.spyOn(API, "getTemplateVersionVariables");
		const examples = vi.spyOn(API, "getTemplateExamples");
		const bases = vi.spyOn(API, "getTemplateBuilderBases");
		const config = vi.spyOn(API, "getDeploymentConfig");
		const version = vi.spyOn(API, "getTemplateVersionByName");
		renderWithAuth(element, {
			path: route.split("?")[0],
			route,
			...options,
		});
		await screen.findByRole("heading", {
			name: "You don't have permission to view this page",
		});
		expect(file).not.toHaveBeenCalled();
		expect(variables).not.toHaveBeenCalled();
		expect(examples).not.toHaveBeenCalled();
		expect(bases).not.toHaveBeenCalled();
		expect(config).not.toHaveBeenCalled();
		expect(version).not.toHaveBeenCalled();
	},
);

test.each([
	{ path: "", element: <TemplateSettingsPage /> },
	{ path: "schedule", element: <TemplateSchedulePage /> },
	{ path: "variables", element: <TemplateVariablesPage /> },
	{ path: "permissions", element: <TemplatePermissionsPage /> },
])(
	"denies template settings/$path without loading editor data",
	async ({ path, element }) => {
		denyTemplateMutations();
		const acl = vi.spyOn(API, "getTemplateACL");
		const variables = vi.spyOn(API, "getTemplateVersionVariables");
		renderWithTemplateSettingsLayout(element, {
			path: `/templates/:template/settings/${path}`,
			route: `/templates/${MockTemplate.name}/settings/${path}`,
		});
		await screen.findByRole("heading", {
			name: "You don't have permission to view this page",
		});
		expect(acl).not.toHaveBeenCalled();
		expect(variables).not.toHaveBeenCalled();
	},
);

test("template readers can load version metadata without requesting source files", async () => {
	denyTemplateMutations();
	const file = vi.spyOn(API, "getFile");
	const version = vi.spyOn(API, "getTemplateVersionByName");
	renderWithAuth(<TemplateVersionPage />, {
		path: "/templates/:template/versions/:version",
		route: `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}`,
	});
	await screen.findByRole("link", { name: "Create workspace" });
	expect(version).toHaveBeenCalledWith(
		"default",
		MockTemplate.name,
		MockTemplateVersion.name,
	);
	expect(file).not.toHaveBeenCalled();
});

test("template creators can use builder deep links without deployment-config access", async () => {
	server.use(
		http.post<never, AuthorizationRequest>(
			"/api/v2/authcheck",
			async ({ request }) => {
				const { checks } = await request.json();
				return HttpResponse.json(
					Object.fromEntries(
						Object.keys(checks).map((key) => [
							key,
							key !== "viewDeploymentConfig",
						]),
					),
				);
			},
		),
	);
	vi.spyOn(API, "getTemplateBuilderBases").mockResolvedValue({ bases: [] });
	const modules = vi
		.spyOn(API, "getTemplateBuilderModules")
		.mockResolvedValue({ modules: [] });
	vi.spyOn(API, "recordTemplateBuilderSession").mockResolvedValue(undefined);
	const config = vi.spyOn(API, "getDeploymentConfig");
	const { router } = renderWithAuth(<TemplateBuilderPage />, {
		path: "/templates/new/builder",
		route: "/templates/new/builder?base=quickstart",
	});
	await waitFor(() => expect(modules).toHaveBeenCalled());
	expect(new URLSearchParams(router.state.location.search).has("base")).toBe(
		false,
	);
	expect(config).not.toHaveBeenCalled();
});

test("disabled builder deep links return to template upload", async () => {
	const error = mockApiError({ message: "Route not found." });
	vi.spyOn(API, "getTemplateBuilderBases").mockRejectedValue({
		...error,
		response: { ...error.response, status: 404 },
	});
	const modules = vi.spyOn(API, "getTemplateBuilderModules");
	const { router } = renderWithAuth(<TemplateBuilderPage />, {
		path: "/templates/new/builder",
		route: "/templates/new/builder?base=quickstart",
		extraRoutes: [{ path: "/templates/new", element: null }],
	});
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/templates/new"),
	);
	expect(modules).not.toHaveBeenCalled();
});

test("template creators cannot duplicate a source template they cannot update", async () => {
	server.use(
		http.post<never, AuthorizationRequest>(
			"/api/v2/authcheck",
			async ({ request }) => {
				const { checks } = await request.json();
				return HttpResponse.json(
					Object.fromEntries(
						Object.entries(checks).map(([name, check]) => [
							name,
							check.object.resource_type !== "template" ||
								check.action !== "update",
						]),
					),
				);
			},
		),
	);
	const variables = vi.spyOn(API, "getTemplateVersionVariables");
	const version = vi.spyOn(API, "getTemplateVersion");
	renderWithAuth(<CreateTemplatePage />, {
		path: "/templates/new",
		route: `/templates/new?fromTemplate=${MockTemplate.id}`,
	});
	await screen.findByRole("heading", {
		name: "You don't have permission to view this page",
	});
	expect(variables).not.toHaveBeenCalled();
	expect(version).not.toHaveBeenCalled();
});

test("a delete-only template administrator can confirm deletion", async () => {
	const onDelete = vi.fn();
	const remove = vi
		.spyOn(API, "deleteTemplate")
		.mockResolvedValue(MockTemplate);
	vi.spyOn(API, "getWorkspaces").mockResolvedValue({
		workspaces: [],
		count: 0,
	});
	renderWithAuth(
		<TemplatePageHeader
			canCreateTemplates={false}
			template={MockTemplate}
			activeVersion={MockTemplateVersion}
			permissions={{ canUpdateTemplate: false, canDeleteTemplate: true }}
			workspacePermissions={{ createWorkspaceForUserID: false }}
			onDeleteTemplate={onDelete}
		/>,
	);
	const user = userEvent.setup();
	await user.click(await screen.findByRole("button", { name: "Open menu" }));
	await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
	await user.type(
		await screen.findByRole("textbox", {
			name: "Name of the template to delete",
		}),
		MockTemplate.name,
	);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1));
	expect(remove).toHaveBeenCalledWith(MockTemplate.id);
});

test("template editors cannot open prebuild controls when the feature is disabled", async () => {
	vi.spyOn(API, "getEntitlements").mockResolvedValue({
		...MockEntitlements,
		has_license: true,
		features: {
			...MockEntitlements.features,
			workspace_prebuilds: {
				...MockEntitlements.features.workspace_prebuilds,
				enabled: false,
			},
		},
	});
	const invalidate = vi.spyOn(API, "invalidateTemplatePresets");
	renderWithAuth(<TemplateLayout />, {
		path: "/templates/:template",
		route: `/templates/${MockTemplate.name}/prebuilds`,
		children: [{ path: "prebuilds", element: <TemplatePrebuildsPage /> }],
	});
	await screen.findByRole("heading", {
		name: "You don't have permission to view this page",
	});
	expect(invalidate).not.toHaveBeenCalled();
});
test("template editors can invalidate prebuilds when the feature is enabled", async () => {
	vi.spyOn(API, "getEntitlements").mockResolvedValue({
		...MockEntitlements,
		has_license: true,
		features: {
			...MockEntitlements.features,
			workspace_prebuilds: { entitlement: "entitled", enabled: true },
		},
	});
	const invalidate = vi
		.spyOn(API, "invalidateTemplatePresets")
		.mockResolvedValue({ invalidated: [] });
	renderWithAuth(<TemplateLayout />, {
		path: "/templates/:template",
		route: `/templates/${MockTemplate.name}/prebuilds`,
		children: [{ path: "prebuilds", element: <TemplatePrebuildsPage /> }],
	});
	await userEvent.click(
		await screen.findByRole("button", { name: "Invalidate now" }),
	);
	await waitFor(() => expect(invalidate).toHaveBeenCalledWith(MockTemplate.id));
});

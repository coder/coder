import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Permissions } from "#/modules/permissions";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockOrganizationPermissions,
	MockTemplate,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import TemplatesPage from "./TemplatesPage";

const auth: { permissions: Permissions } = {
	permissions: { ...MockNoPermissions, updateAnyTemplate: true },
};

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: auth.permissions,
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
	}),
}));

afterEach(() => {
	vi.restoreAllMocks();
	auth.permissions = { ...MockNoPermissions, updateAnyTemplate: true };
});

const renderTemplatesPage = () =>
	renderWithRouter(
		createMemoryRouter(
			[{ path: "/ai/settings/templates", element: <TemplatesPage /> }],
			{ initialEntries: ["/ai/settings/templates"] },
		),
	);

it("requests server-side filtered templates after searching", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[`${MockDefaultOrganization.id}.updateTemplates`]: true,
	});
	const templatesSpy = vi.spyOn(API, "getTemplates").mockResolvedValue([
		MockTemplate,
		{
			...MockTemplate,
			id: "second-template",
			display_name: "Second Template",
		},
	]);
	const usersSpy = vi.spyOn(API, "getUsers");
	renderTemplatesPage();

	await waitFor(() => expect(templatesSpy).toHaveBeenCalledWith({ q: "" }));
	await user.type(
		screen.getByRole("combobox", { name: "Search and filter templates…" }),
		"Second",
	);

	await waitFor(() =>
		expect(templatesSpy).toHaveBeenCalledWith({ q: "Second" }),
	);
	expect(usersSpy).not.toHaveBeenCalled();
});

it("sends independent updates for concurrently toggled templates", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[`${MockDefaultOrganization.id}.updateTemplates`]: true,
	});
	const secondTemplate = {
		...MockTemplate,
		id: "second-template",
		display_name: "Second Template",
	};
	vi.spyOn(API, "getTemplates").mockResolvedValue([
		MockTemplate,
		secondTemplate,
	]);
	const firstUpdate = createDeferred<typeof MockTemplate | null>();
	const secondUpdate = createDeferred<typeof MockTemplate | null>();
	const updateSpy = vi
		.spyOn(API, "updateTemplateMeta")
		.mockReturnValueOnce(firstUpdate.promise)
		.mockReturnValueOnce(secondUpdate.promise);
	renderTemplatesPage();

	await user.click(
		await screen.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Test Template in My Organization",
		}),
	);
	await user.click(
		screen.getByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Second Template in My Organization",
		}),
	);

	expect(updateSpy).toHaveBeenNthCalledWith(1, MockTemplate.id, {
		agents_allowed: false,
	});
	expect(updateSpy).toHaveBeenNthCalledWith(2, secondTemplate.id, {
		agents_allowed: false,
	});
	firstUpdate.resolve({ ...MockTemplate, agents_allowed: false });
	secondUpdate.resolve({ ...secondTemplate, agents_allowed: false });
});

it("does not request templates without permission to manage them", async () => {
	auth.permissions = MockNoPermissions;
	const permissionsSpy = vi.spyOn(API, "checkAuthorization");
	const templatesSpy = vi.spyOn(API, "getTemplates");
	renderTemplatesPage();

	await screen.findByText("You don't have permission to view this page");
	expect(permissionsSpy).not.toHaveBeenCalled();
	expect(templatesSpy).not.toHaveBeenCalled();
});

it("waits for organization permissions before requesting templates", async () => {
	let resolvePermissions: (value: Record<string, boolean>) => void = () => {};
	const permissionsSpy = vi.spyOn(API, "checkAuthorization").mockImplementation(
		() =>
			new Promise((resolve) => {
				resolvePermissions = resolve;
			}),
	);
	const templatesSpy = vi
		.spyOn(API, "getTemplates")
		.mockResolvedValue([MockTemplate]);
	renderTemplatesPage();

	await waitFor(() => expect(permissionsSpy).toHaveBeenCalled());
	expect(templatesSpy).not.toHaveBeenCalled();
	resolvePermissions({
		[`${MockDefaultOrganization.id}.updateTemplates`]:
			MockOrganizationPermissions.updateTemplates,
	});
	await waitFor(() => expect(templatesSpy).toHaveBeenCalledWith({ q: "" }));
});

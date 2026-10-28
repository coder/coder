import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { foundationPermissionsKey } from "#/api/queries/foundations";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import {
	MockFoundation,
	MockFoundationPermissions,
} from "#/testHelpers/templateResources";
import CreateFoundationPage from "./CreateFoundationPage";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		experiments: ["template-resources"],
		organizations: [MockDefaultOrganization, MockOrganization2],
	}),
}));

beforeEach(() => {
	vi.spyOn(API, "checkAuthorization").mockResolvedValue(
		MockFoundationPermissions,
	);
});
afterEach(() => vi.restoreAllMocks());

const renderPage = () => {
	const router = createMemoryRouter(
		[
			{ path: "/foundations/new", element: <CreateFoundationPage /> },
			{
				path: "/organizations/:organization/foundations/:foundation",
				element: <div />,
			},
		],
		{
			initialEntries: [`/foundations/new?organization=${MockOrganization2.id}`],
		},
	);
	return { ...renderWithRouter(router), router };
};

it.each([true, false])(
	"creates in the preselected organization with workspace destruction protection=%s without binding",
	async (preventDestroy) => {
		const user = userEvent.setup();
		const create = vi.spyOn(API, "createFoundation").mockResolvedValue({
			...MockFoundation,
			organization_id: MockOrganization2.id,
		});
		const bind = vi.spyOn(API, "setTemplateFoundation");
		const { router, queryClient } = renderPage();
		await user.type(
			screen.getByRole("textbox", { name: "Foundation name" }),
			"shared-network",
		);
		await waitFor(() => expect(API.checkAuthorization).toHaveBeenCalled());
		await act(async () => {
			queryClient.setQueryData(foundationPermissionsKey(MockOrganization2.id), {
				...MockFoundationPermissions,
			});
		});
		if (!preventDestroy)
			await user.click(
				screen.getByRole("checkbox", {
					name: "Prevent Destroy while workspaces exist",
				}),
			);
		await user.click(screen.getByRole("button", { name: "Create Foundation" }));
		await waitFor(() =>
			expect(create).toHaveBeenCalledWith(MockOrganization2.id, {
				name: "shared-network",
				provisioner_tags: {},
				prevent_destroy_with_workspaces: preventDestroy,
			}),
		);
		await waitFor(() =>
			expect(router.state.location.pathname).toBe(
				`/organizations/${MockOrganization2.id}/foundations/${MockFoundation.id}`,
			),
		);
		expect(router.state.location.search).toBe("?edit=true");
		expect(bind).not.toHaveBeenCalled();
	},
);

it("does not create without organization permission", async () => {
	vi.mocked(API.checkAuthorization).mockResolvedValue({
		...MockFoundationPermissions,
		canCreateFoundation: false,
	});
	const create = vi.spyOn(API, "createFoundation");
	const user = userEvent.setup();
	renderPage();
	await user.type(
		screen.getByRole("textbox", { name: "Foundation name" }),
		"blocked",
	);
	await waitFor(() => expect(API.checkAuthorization).toHaveBeenCalled());
	await user.click(screen.getByRole("button", { name: "Create Foundation" }));
	expect(create).not.toHaveBeenCalled();
});

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { foundationsKey } from "#/api/queries/foundations";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
import { MockFoundation } from "#/testHelpers/templateResources";
import { FoundationBinding } from "./FoundationBinding";

const mockUnbound = { ...MockFoundation, template_id: null };
beforeEach(() => {
	vi.spyOn(API, "getFoundations").mockResolvedValue([mockUnbound]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		canUseFoundation: true,
	});
});
afterEach(() => vi.restoreAllMocks());

it.each(["authcheck", "attachment"])(
	"retains the existing Foundation selection after %s failure and retries without running Terraform",
	async (failure) => {
		const user = userEvent.setup();
		const create = vi.spyOn(API, "createFoundation");
		const run = vi.spyOn(API, "createFoundationRun");
		const bind = vi
			.spyOn(API, "setTemplateFoundation")
			.mockResolvedValue(MockFoundation);
		if (failure === "authcheck") {
			let fail = true;
			vi.mocked(API.checkAuthorization).mockImplementation(
				async ({ checks }) => {
					if (checks.canUseFoundation && fail) {
						fail = false;
						throw mockApiError({ message: "Permissions unavailable." });
					}
					return { canUseFoundation: true };
				},
			);
		} else {
			bind.mockRejectedValueOnce(
				mockApiError({ message: "Attachment failed." }),
			);
		}
		const { queryClient } = render(
			<FoundationBinding template={MockTemplate} canCreate={false} canBind />,
		);
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");
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
		await user.click(
			await screen.findByRole("button", { name: "Retry attachment" }),
		);
		await waitFor(() =>
			expect(bind).toHaveBeenCalledTimes(failure === "authcheck" ? 1 : 2),
		);
		expect(bind).toHaveBeenLastCalledWith(MockTemplate.id, {
			foundation_id: mockUnbound.id,
		});
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: foundationsKey(MockTemplate.organization_id),
			exact: true,
		});
		expect(create).not.toHaveBeenCalled();
		expect(run).not.toHaveBeenCalled();
	},
);

it("navigates to standalone creation with the organization even without template update permission", async () => {
	const user = userEvent.setup();
	const create = vi.spyOn(API, "createFoundation");
	const bind = vi.spyOn(API, "setTemplateFoundation");
	const { router } = renderWithRouter(
		createMemoryRouter([
			{
				path: "/",
				element: (
					<FoundationBinding
						template={MockTemplate}
						canCreate
						canBind={false}
					/>
				),
			},
			{ path: "/foundations/new", element: <div /> },
		]),
	);
	await user.click(screen.getByRole("link", { name: "Create Foundation" }));
	expect(router.state.location.pathname).toBe("/foundations/new");
	expect(router.state.location.search).toBe(
		`?organization=${MockTemplate.organization_id}`,
	);
	expect(create).not.toHaveBeenCalled();
	expect(bind).not.toHaveBeenCalled();
});

it("requires use permission on the selected Foundation", async () => {
	vi.mocked(API.checkAuthorization).mockResolvedValue({
		canUseFoundation: false,
	});
	const bind = vi.spyOn(API, "setTemplateFoundation");
	const user = userEvent.setup();
	render(
		<FoundationBinding template={MockTemplate} canCreate={false} canBind />,
	);
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
	await screen.findByRole("alert");
	expect(bind).not.toHaveBeenCalled();
});

it("retries the list after an initial failure", async () => {
	vi.mocked(API.getFoundations).mockRejectedValueOnce(
		mockApiError({ message: "List unavailable." }),
	);
	const user = userEvent.setup();
	render(<FoundationBinding template={MockTemplate} canCreate canBind />);
	await user.click(
		await screen.findByRole("button", { name: "Retry Foundations" }),
	);
	await waitFor(() => expect(API.getFoundations).toHaveBeenCalledTimes(2));
});

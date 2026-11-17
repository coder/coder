import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useSearchParams } from "react-router";
import { API } from "#/api/api";
import { useFilter } from "#/components/Filter/Filter";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { AuditFilter } from "./AuditFilter";

const Harness = ({ onChange }: { onChange: (query: string) => void }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: onChange,
	});
	return <AuditFilter filter={filter} />;
};

describe("AuditFilter", () => {
	afterEach(() => vi.restoreAllMocks());
	it.each([
		["Created workspaces", "resource_type:workspace action:create"],
		["Added templates", "resource_type:template action:create"],
		["Deleted users", "resource_type:user action:delete"],
		[
			"Builds started by a user",
			"resource_type:workspace_build action:start build_reason:initiator",
		],
		["User logins", "resource_type:api_key action:login"],
	])("applies the %s preset as the complete query", async (label, query) => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=action:write%20date_from:2026-01-01",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: label }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(query));
	});

	it("applies a preset found by typing its name", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.type(
			await screen.findByRole("combobox", {
				name: "Search and filter audit logs…",
			}),
			"User logins",
		);
		await user.click(
			await screen.findByRole("option", { name: "User logins" }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"resource_type:api_key action:login",
			),
		);
	});

	it("applies an inline preset using the keyboard", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await screen.findByRole("option", { name: "User logins" });
		await user.keyboard("{End}{Enter}");
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"resource_type:api_key action:login",
			),
		);
	});

	it.each([
		["resource_type:", "Experiment Rule", "resource_type:experiment_rule"],
		[
			"resource_type:",
			"Chat Instruction Settings",
			"resource_type:chat_instruction_settings",
		],
		[
			"resource_type:",
			"Chat Operational Settings",
			"resource_type:chat_operational_settings",
		],
		["action:", "Create", "action:create"],
		["build_reason:", "API", "build_reason:initiator"],
	])("selects %s %s", async (prefix, label, query) => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.type(
			await screen.findByRole("combobox", {
				name: "Search and filter audit logs…",
			}),
			prefix,
		);
		await user.click(await screen.findByRole("option", { name: label }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(query));
	});

	it.each(["action:", "delete"])(
		"replaces a scalar action when selecting from %s",
		async (input) => {
			const user = userEvent.setup();
			const onChange = vi.fn();
			renderWithAuth(<Harness onChange={onChange} />, {
				route: "/?filter=resource_type:workspace%20action:create",
			});
			await user.type(
				await screen.findByRole("combobox", {
					name: "Search and filter audit logs…",
				}),
				input,
			);
			await user.click(await screen.findByRole("option", { name: "Delete" }));
			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith(
					"resource_type:workspace action:delete",
				),
			);
		},
	);

	it("preserves advanced query fields when removing a chip from a filter URL", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route:
				"/?filter=resource_type:workspace%20action:create%20date_from:2026-01-01",
		});
		await user.click(
			await screen.findByRole("button", { name: /remove.*create/i }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"resource_type:workspace date_from:2026-01-01",
			),
		);
	});

	it("keeps historical action queries usable", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=action:connect%20resource_type:workspace",
		});
		await user.click(
			await screen.findByRole("button", { name: /remove.*workspace/i }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("action:connect"),
		);
	});

	it("offers only organizations where audit logs can be read", async () => {
		vi.spyOn(API, "getOrganizations").mockResolvedValue([
			MockDefaultOrganization,
			MockOrganization2,
		]);
		const checkAuthorization = vi
			.spyOn(API, "checkAuthorization")
			.mockResolvedValue({
				[MockDefaultOrganization.id]: true,
				[MockOrganization2.id]: false,
			});
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.type(
			await screen.findByRole("combobox", {
				name: "Search and filter audit logs…",
			}),
			"organization:",
		);
		await screen.findByRole("option", {
			name: MockDefaultOrganization.display_name,
		});
		await user.keyboard("{End}{Enter}");
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				`organization:${MockDefaultOrganization.name}`,
			),
		);
		expect(checkAuthorization).toHaveBeenCalledWith({
			checks: {
				[MockDefaultOrganization.id]: {
					object: {
						resource_type: "audit_log",
						organization_id: MockDefaultOrganization.id,
					},
					action: "read",
				},
				[MockOrganization2.id]: {
					object: {
						resource_type: "audit_log",
						organization_id: MockOrganization2.id,
					},
					action: "read",
				},
			},
		});
	});

	it("suggests the current user without listing users when permission is unavailable", async () => {
		server.use(
			http.post("/api/v2/authcheck", () =>
				HttpResponse.json(MockNoPermissions),
			),
		);
		const getUsers = vi.fn();
		server.use(
			http.get("/api/v2/users", () => {
				getUsers();
				return HttpResponse.json({ users: [], count: 0 });
			}),
		);
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.type(
			await screen.findByRole("combobox", {
				name: "Search and filter audit logs…",
			}),
			"username:",
		);
		await user.click(await screen.findByRole("option", { name: /\(you\)/ }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("username:me"),
		);
		expect(getUsers).not.toHaveBeenCalled();
	});
});

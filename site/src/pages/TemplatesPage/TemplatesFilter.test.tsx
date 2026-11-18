import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useSearchParams } from "react-router";
import { useFilter } from "#/components/Filter/Filter";
import { WorkspacesFilter } from "#/pages/WorkspacesPage/filter/WorkspacesFilter";
import { MockNoPermissions } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { TemplatesFilter } from "./TemplatesFilter";

const Harness = ({
	onChange,
	page = "templates",
}: {
	onChange: (query: string) => void;
	page?: "templates" | "workspaces";
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: (params) => {
			onChange(params.get("filter") ?? "");
			setSearchParams(params);
		},
	});
	return page === "workspaces" ? (
		<WorkspacesFilter filter={filter} error={undefined} />
	) : (
		<TemplatesFilter filter={filter} />
	);
};

describe("TemplatesFilter", () => {
	it.each([
		["Deprecated", "deprecated:true"],
		["Compatibility mode", "compatibility_mode:true"],
	])("filters by %s", async (label, query) => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />);
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: label }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(query));
	});

	it("keeps template options separate from cached workspace attributes", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		const { queryClient, router } = renderWithAuth(
			<Harness page="workspaces" onChange={onChange} />,
			{
				path: "/workspaces",
				route: "/workspaces",
				extraRoutes: [
					{ path: "/templates", element: <Harness onChange={onChange} /> },
				],
			},
		);
		queryClient.setDefaultOptions({
			queries: {
				...queryClient.getDefaultOptions().queries,
				staleTime: Number.POSITIVE_INFINITY,
				gcTime: Number.POSITIVE_INFINITY,
			},
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: "Outdated" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("outdated:true"),
		);

		await act(() => router.navigate("/templates"));
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: "Deprecated" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("deprecated:true"),
		);
	});

	it("combines attributes and removes a chip from an existing filter URL", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=compatibility_mode:true",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: "Deprecated" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"compatibility_mode:true deprecated:true",
			),
		);
		await user.click(
			screen.getByRole("button", { name: /remove.*compatibility/i }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("deprecated:true"),
		);
	});

	it.each([true, false])(
		"loads authors on demand with list permission %s",
		async (viewAllUsers) => {
			server.use(
				http.post("/api/v2/authcheck", () =>
					HttpResponse.json({ ...MockNoPermissions, viewAllUsers }),
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
			const filtersButton = await screen.findByRole("button", {
				name: "Filters",
			});
			expect(getUsers).not.toHaveBeenCalled();
			await user.click(filtersButton);
			await screen.findByRole("option", { name: "Author" });
			await user.keyboard("{Home}{ArrowRight}");
			await user.click(await screen.findByRole("option", { name: /\(you\)/ }));
			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("author:me"),
			);
			if (viewAllUsers) {
				expect(getUsers).toHaveBeenCalled();
			} else {
				expect(getUsers).not.toHaveBeenCalled();
			}
		},
	);
});

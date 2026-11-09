import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useSearchParams } from "react-router";
import { filterComboboxOptions } from "#/components/Filter/FilterCombobox/queries";
import { MockNoPermissions } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { TemplatesFilter, useTemplatesFilter } from "./TemplatesFilter";

const Harness = ({ onChange }: { onChange: (query: string) => void }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const { filter } = useTemplatesFilter({
		searchParams,
		onSearchParamsChange: (params) => {
			onChange(params.get("filter") ?? "");
			setSearchParams(params);
		},
	});
	return <TemplatesFilter filter={filter} />;
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
		const { queryClient } = renderWithAuth(<Harness onChange={onChange} />);
		const { queryKey } = filterComboboxOptions(
			"attribute",
			undefined,
			"",
			true,
		);
		queryClient.setQueryDefaults(queryKey, {
			staleTime: Number.POSITIVE_INFINITY,
		});
		await act(async () => {
			queryClient.setQueryData(queryKey, [
				{ label: "Dormant", value: "dormant", token: "dormant:true" },
			]);
		});
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

	it("lets users without list permission filter by themselves without fetching users", async () => {
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
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await screen.findByRole("option", { name: "Author" });
		await user.keyboard("{Home}{ArrowRight}");
		await user.click(await screen.findByRole("option", { name: /\(you\)/ }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("author:me"));
		expect(getUsers).not.toHaveBeenCalled();
	});
});

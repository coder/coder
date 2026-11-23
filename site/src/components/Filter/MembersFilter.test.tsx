import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSearchParams } from "react-router";
import { useFilter } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MembersFilter } from "./MembersFilter";

const Harness = ({
	onChange,
	onSearchParamsChange,
}: {
	onChange: (query: string) => void;
	onSearchParamsChange?: (params: URLSearchParams) => void;
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: onSearchParamsChange ?? setSearchParams,
		onUpdate: onChange,
	});
	return <MembersFilter filter={filter} />;
};

describe("MembersFilter", () => {
	it("adds a status while preserving advanced query fields", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=role:organization-admin",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: "Suspended" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"status:suspended role:organization-admin",
			),
		);
	});

	it.each(["settled", "pending"])(
		"toggles service accounts while preserving status when navigation is %s",
		async (navigation) => {
			const user = userEvent.setup();
			const onChange = vi.fn();
			const pendingNavigations: string[] = [];
			const { router } = renderWithAuth(
				<Harness
					onChange={onChange}
					onSearchParamsChange={
						navigation === "pending"
							? (params) => {
									// Snapshot each request without letting navigation refresh the hook.
									pendingNavigations.push(params.toString());
								}
							: undefined
					}
				/>,
				{ route: "/?filter=status:active&page=2" },
			);
			await user.click(await screen.findByRole("button", { name: "Filters" }));
			await user.click(
				await screen.findByRole("option", { name: "Service account" }),
			);
			expect(onChange).toHaveBeenCalledExactlyOnceWith(
				"status:active service_account:true",
			);

			if (navigation === "settled") {
				await waitFor(() =>
					expect(
						new URLSearchParams(router.state.location.search).get("filter"),
					).toBe("status:active service_account:true"),
				);
			} else {
				expect(router.state.location.search).toBe(
					"?filter=status:active&page=2",
				);
			}

			await user.click(
				await screen.findByRole("option", { name: "Service account" }),
			);
			expect(onChange.mock.calls).toEqual([
				["status:active service_account:true"],
				["status:active"],
			]);

			if (navigation === "pending") {
				expect(
					pendingNavigations.map((search) =>
						Object.fromEntries(new URLSearchParams(search)),
					),
				).toEqual([
					{ filter: "status:active service_account:true", page: "2" },
					{ filter: "status:active", page: "2" },
				]);
				await act(() => router.navigate(`?${pendingNavigations[1]}`));
			}

			await waitFor(() => {
				expect(router.state.navigation.state).toBe("idle");
				expect(
					Object.fromEntries(new URLSearchParams(router.state.location.search)),
				).toEqual({ filter: "status:active", page: "2" });
			});
		},
	);

	it("removes a status chip while preserving free text", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=status:active%20alice",
		});
		await user.click(
			await screen.findByRole("button", { name: "Remove status:active" }),
		);
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("alice"));
	});
});

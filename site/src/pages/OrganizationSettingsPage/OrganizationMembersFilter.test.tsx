import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSearchParams } from "react-router";
import { useFilter } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { OrganizationMembersFilter } from "./OrganizationMembersFilter";

const Harness = ({ onChange }: { onChange: (query: string) => void }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: onChange,
	});
	return <OrganizationMembersFilter filter={filter} />;
};

describe("OrganizationMembersFilter", () => {
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

	it("toggles service accounts while preserving status", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=status:active",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(
			await screen.findByRole("option", { name: "Service account" }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"status:active service_account:true",
			),
		);
		await user.click(
			await screen.findByRole("option", { name: "Service account" }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:active"),
		);
	});

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

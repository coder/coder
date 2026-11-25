import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockToken } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { TokensPageView } from "./TokensPageView";

it("allows deleting a cached token while its list refetches", async () => {
	const onDelete = vi.fn();
	renderWithAuth(
		<TokensPageView
			tokens={[MockToken]}
			isFetching
			hasLoaded
			onDelete={onDelete}
		/>,
	);

	await userEvent.click(
		await screen.findByRole("button", { name: "Delete token" }),
	);
	expect(onDelete).toHaveBeenCalledWith(MockToken);
});

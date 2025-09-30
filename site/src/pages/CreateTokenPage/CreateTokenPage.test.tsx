import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type { ScopeCatalog } from "api/typesGenerated";
import CreateTokenPage from "./CreateTokenPage";
import { NANO_HOUR } from "./utils";

describe("TokenPage", () => {
	beforeEach(() => {
		jest.resetAllMocks();
	});

	it("shows the success modal", async () => {
		const catalog: ScopeCatalog = {
			specials: ["coder:all"],
			low_level: [
				{
					name: "workspace:read",
					resource: "workspace",
					action: "read",
				},
			],
			composites: [
				{
					name: "coder:workspaces.access",
					expands_to: ["workspace:read"],
				},
			],
		};
		jest.spyOn(API, "getScopeCatalog").mockResolvedValueOnce(catalog);
		jest.spyOn(API, "getTokenConfig").mockResolvedValueOnce({
			max_token_lifetime: 90 * 24 * NANO_HOUR,
		});
		jest.spyOn(API, "createToken").mockResolvedValueOnce({
			key: "abcd",
		});

		// When
		const { container } = renderWithAuth(<CreateTokenPage />, {
			route: "/settings/tokens/new",
			path: "/settings/tokens/new",
		});
		await waitForLoaderToBeRemoved();

		const form = container.querySelector("form") as HTMLFormElement;
		await act(async () => {
			await userEvent.type(screen.getByLabelText(/Name/), "my-token");
		});
		await act(async () => {
			await userEvent.click(
				within(form).getByRole("button", { name: /create token/i }),
			);
		});

		// Then
		expect(screen.getByText("abcd")).toBeInTheDocument();
	});
});

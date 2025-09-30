import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type { APIKey, ScopeCatalog } from "api/typesGenerated";
import * as reactRouter from "react-router";
import { NANO_HOUR } from "../CreateTokenPage/utils";
import EditTokenPage from "./EditTokenPage";

const makeToken = (overrides: Partial<APIKey> = {}): APIKey => ({
	id: "token-id",
	user_id: "user-1",
	last_used: new Date().toISOString(),
	expires_at: new Date().toISOString(),
	created_at: new Date().toISOString(),
	updated_at: new Date().toISOString(),
	login_type: "token",
	scope: "all",
	scopes: ["coder:workspaces.access"],
	token_name: "ci-bot",
	lifetime_seconds: 30 * 24 * 60 * 60,
	allow_list: [],
	...overrides,
});

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

describe("EditTokenPage", () => {
	beforeEach(() => {
		jest.resetAllMocks();
	});

	it("updates a token", async () => {
		const navigateMock = jest.fn();
		jest.spyOn(reactRouter, "useNavigate").mockReturnValue(navigateMock);
		jest.spyOn(API, "getToken").mockResolvedValue(makeToken());
		jest.spyOn(API, "getScopeCatalog").mockResolvedValue(catalog);
		jest.spyOn(API, "getTokenConfig").mockResolvedValue({
			max_token_lifetime: 90 * 24 * NANO_HOUR,
		});
		const updateSpy = jest
			.spyOn(API, "updateToken")
			.mockResolvedValue(makeToken({ lifetime_seconds: 60 * 24 * 60 * 60 }));

		const user = userEvent.setup();

		renderWithAuth(<EditTokenPage />, {
			route: "/settings/tokens/ci-bot/edit",
			path: "/settings/tokens/:tokenName/edit",
		});

		await waitForLoaderToBeRemoved();

		await act(async () => {
			await user.click(screen.getByLabelText(/Lifetime/i));
		});
		await act(async () => {
			await user.click(screen.getByRole("option", { name: "60 days" }));
		});

		await act(async () => {
			await user.click(screen.getByRole("button", { name: /save changes/i }));
		});

		await waitFor(() => expect(updateSpy).toHaveBeenCalled());

		expect(updateSpy).toHaveBeenCalledWith(
			"ci-bot",
			expect.objectContaining({ lifetime: 60 * 24 * NANO_HOUR }),
		);
		expect(navigateMock).toHaveBeenCalledWith("/settings/tokens");
	});

	it("preloads existing allow-list entries", async () => {
		jest.spyOn(reactRouter, "useNavigate").mockReturnValue(jest.fn());
		jest.spyOn(API, "getToken").mockResolvedValue(
			makeToken({
				allow_list: [
					{ type: "workspace", id: "*", display_name: "All workspaces" },
					{ type: "user", id: "user-123" },
				],
				lifetime_seconds: 7 * 24 * 60 * 60,
			}),
		);
		jest.spyOn(API, "getScopeCatalog").mockResolvedValue(catalog);
		jest.spyOn(API, "getTokenConfig").mockResolvedValue({
			max_token_lifetime: 90 * 24 * NANO_HOUR,
		});

		renderWithAuth(<EditTokenPage />, {
			route: "/settings/tokens/ci-bot/edit",
			path: "/settings/tokens/:tokenName/edit",
		});

		await waitForLoaderToBeRemoved();

		await waitFor(() => {
			expect(
				screen.getByText("workspace : All workspaces"),
			).toBeInTheDocument();
			expect(screen.getByText("user : user-123")).toBeInTheDocument();
		});
	});
});

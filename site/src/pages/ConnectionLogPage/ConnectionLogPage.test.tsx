import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockEntitlementsWithConnectionLog,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils";
import { HttpResponse, http } from "msw";
import * as CreateDayString from "utils/createDayString";
import ConnectionLogPage from "./ConnectionLogPage";

interface RenderPageOptions {
	filter?: string;
	page?: number;
}

const renderPage = async ({ filter, page }: RenderPageOptions = {}) => {
	let route = "/connectionlog";
	const params = new URLSearchParams();

	if (filter) {
		params.set("filter", filter);
	}

	if (page) {
		params.set("page", page.toString());
	}

	if (Array.from(params).length > 0) {
		route += `?${params.toString()}`;
	}

	renderWithAuth(<ConnectionLogPage />, {
		route,
		path: "/connectionlog",
	});
	await waitForLoaderToBeRemoved();
};

describe("ConnectionLogPage", () => {
	beforeEach(() => {
		// Mocking the dayjs module within the createDayString file
		const mock = vi.spyOn(CreateDayString, "createDayString");
		mock.mockImplementation(() => "a minute ago");

		// Mock the entitlements
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithConnectionLog);
			}),
		);
	});

	it("renders page 5", async () => {
		// Given
		const page = 5;
		const getConnectionLogsSpy = vi
			.spyOn(API, "getConnectionLogs")
			.mockResolvedValue({
				connection_logs: [
					MockConnectedSSHConnectionLog,
					MockDisconnectedSSHConnectionLog,
				],
				count: 2,
			});

		// When
		await renderPage({ page: page });

		// Then
		expect(getConnectionLogsSpy).toHaveBeenCalledWith({
			limit: DEFAULT_RECORDS_PER_PAGE,
			offset: DEFAULT_RECORDS_PER_PAGE * (page - 1),
			q: "",
		});
		screen.getByTestId(
			`connection-log-row-${MockConnectedSSHConnectionLog.id}`,
		);
		screen.getByTestId(
			`connection-log-row-${MockDisconnectedSSHConnectionLog.id}`,
		);
	});

	describe("Filtering", () => {
		it("filters by URL", async () => {
			const getConnectionLogsSpy = vi
				.spyOn(API, "getConnectionLogs")
				.mockResolvedValue({
					connection_logs: [MockConnectedSSHConnectionLog],
					count: 1,
				});

			const query = "type:ssh status:ongoing";
			await renderPage({ filter: query });

			expect(getConnectionLogsSpy).toHaveBeenCalledWith({
				limit: DEFAULT_RECORDS_PER_PAGE,
				offset: 0,
				q: query,
			});
		});

		it("resets page to 1 when filter is changed", async () => {
			await renderPage({ page: 2 });

			const getConnectionLogsSpy = vi.spyOn(API, "getConnectionLogs");
			getConnectionLogsSpy.mockClear();

			const filterField = screen.getByLabelText("Filter");
			const query = "type:ssh status:ongoing";
			await userEvent.type(filterField, query);

			await waitFor(() =>
				expect(getConnectionLogsSpy).toHaveBeenCalledWith({
					limit: DEFAULT_RECORDS_PER_PAGE,
					offset: 0,
					q: query,
				}),
			);
		});
	});
});

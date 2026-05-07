import {
	createEvent,
	fireEvent,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { API } from "#/api/api";
import type { AuditLogsRequest } from "#/api/typesGenerated";
import { DEFAULT_RECORDS_PER_PAGE } from "#/components/PaginationWidget/utils";
import {
	MockAuditLog,
	MockAuditLog2,
	MockEntitlementsWithAuditLog,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import * as CreateDayString from "#/utils/createDayString";
import AuditPage from "./AuditPage";

interface RenderPageOptions {
	filter?: string;
	page?: number;
}

const renderPage = async ({ filter, page }: RenderPageOptions = {}) => {
	let route = "/audit";
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

	renderWithAuth(<AuditPage />, {
		route,
		path: "/audit",
	});
	await waitForLoaderToBeRemoved();
};

describe("AuditPage", () => {
	beforeEach(() => {
		// Mocking the dayjs module within the createDayString file
		const mock = vi.spyOn(CreateDayString, "createDayString");
		mock.mockImplementation(() => "a minute ago");

		// Mock the entitlements
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithAuditLog);
			}),
		);
	});

	it("renders page 5", async () => {
		// Given
		const page = 5;
		const getAuditLogsSpy = vi.spyOn(API, "getAuditLogs").mockResolvedValue({
			audit_logs: [MockAuditLog, MockAuditLog2],
			count: 2,
			count_cap: 0,
		});

		// When
		await renderPage({ page: page });

		// Then
		expect(getAuditLogsSpy).toBeCalledWith({
			limit: DEFAULT_RECORDS_PER_PAGE,
			offset: DEFAULT_RECORDS_PER_PAGE * (page - 1),
			q: "",
		});
		screen.getByTestId(`audit-log-row-${MockAuditLog.id}`);
		screen.getByTestId(`audit-log-row-${MockAuditLog2.id}`);
	});

	it("toggles an expandable audit row with Enter", async () => {
		vi.spyOn(API, "getAuditLogs").mockResolvedValue({
			audit_logs: [MockAuditLog],
			count: 1,
			count_cap: 0,
		});

		await renderPage();

		const row = screen.getByTestId(`audit-log-row-${MockAuditLog.id}`);
		const expandableRowButton = within(row).getByRole("button");

		expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();

		fireEvent.keyDown(expandableRowButton, { key: "Enter" });

		expect(screen.getAllByText(/ttl:/i)).toHaveLength(2);

		fireEvent.keyDown(expandableRowButton, { key: "Enter" });

		await waitFor(() => {
			expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();
		});
	});

	it("toggles an expandable audit row with Space and prevents default", async () => {
		vi.spyOn(API, "getAuditLogs").mockResolvedValue({
			audit_logs: [MockAuditLog],
			count: 1,
			count_cap: 0,
		});

		await renderPage();

		const row = screen.getByTestId(`audit-log-row-${MockAuditLog.id}`);
		const expandableRowButton = within(row).getByRole("button");
		const spaceEvent = createEvent.keyDown(expandableRowButton, {
			key: " ",
			code: "Space",
		});
		const preventDefaultSpy = vi.spyOn(spaceEvent, "preventDefault");

		fireEvent(expandableRowButton, spaceEvent);

		expect(preventDefaultSpy).toHaveBeenCalled();
		expect(screen.getAllByText(/ttl:/i)).toHaveLength(2);

		fireEvent.keyDown(expandableRowButton, { key: " " });

		await waitFor(() => {
			expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();
		});
	});

	describe("Filtering", () => {
		it("filters by URL", async () => {
			const getAuditLogsSpy = vi.spyOn(API, "getAuditLogs").mockResolvedValue({
				audit_logs: [MockAuditLog],
				count: 1,
				count_cap: 0,
			});

			const query = "resource_type:workspace action:create";
			await renderPage({ filter: query });

			expect(getAuditLogsSpy).toBeCalledWith({
				limit: DEFAULT_RECORDS_PER_PAGE,
				offset: 0,
				q: query,
			});
		});

		it("resets page to 1 when filter is changed", async () => {
			await renderPage({ page: 2 });

			const getAuditLogsSpy = vi.spyOn(API, "getAuditLogs");
			getAuditLogsSpy.mockClear();

			const filterField = screen.getByLabelText("Filter");
			const query = "resource_type:workspace action:create";
			await userEvent.type(filterField, query);

			await waitFor(() =>
				expect(getAuditLogsSpy).toHaveBeenCalledWith<[AuditLogsRequest]>({
					limit: DEFAULT_RECORDS_PER_PAGE,
					offset: 0,
					q: query,
				}),
			);
		});
	});

	describe("Capped count", () => {
		it("shows capped count indicator and navigates to next page with correct offset", async () => {
			vi.spyOn(API, "getAuditLogs").mockResolvedValue({
				audit_logs: [MockAuditLog, MockAuditLog2],
				count: 2001,
				count_cap: 2000,
			});

			const user = userEvent.setup();
			await renderPage();

			await screen.findByText(/2,000\+/);

			await user.click(screen.getByRole("button", { name: /next page/i }));

			await waitFor(() =>
				expect(API.getAuditLogs).toHaveBeenLastCalledWith<[AuditLogsRequest]>({
					limit: DEFAULT_RECORDS_PER_PAGE,
					offset: DEFAULT_RECORDS_PER_PAGE,
					q: "",
				}),
			);
		});
	});
});

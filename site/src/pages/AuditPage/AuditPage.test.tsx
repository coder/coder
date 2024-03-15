import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { rest } from "msw";
import * as API from "api/api";
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils";
import {
  MockAuditLog,
  MockAuditLog2,
  MockEntitlementsWithAuditLog,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import * as CreateDayString from "utils/createDayString";
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
    const mock = jest.spyOn(CreateDayString, "createDayString");
    mock.mockImplementation(() => "a minute ago");

    // Mock the entitlements
    server.use(
      rest.get("/api/v2/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog));
      }),
    );
  });

  it("renders page 5", async () => {
    // Given
    const page = 5;
    const getAuditLogsSpy = jest.spyOn(API, "getAuditLogs").mockResolvedValue({
      audit_logs: [MockAuditLog, MockAuditLog2],
      count: 2,
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

  describe("Filtering", () => {
    it("filters by URL", async () => {
      const getAuditLogsSpy = jest
        .spyOn(API, "getAuditLogs")
        .mockResolvedValue({ audit_logs: [MockAuditLog], count: 1 });

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

      const getAuditLogsSpy = jest.spyOn(API, "getAuditLogs");
      getAuditLogsSpy.mockClear();

      const filterField = screen.getByLabelText("Filter");
      const query = "resource_type:workspace action:create";
      await userEvent.type(filterField, query);

      await waitFor(() =>
        expect(getAuditLogsSpy).toBeCalledWith({
          limit: DEFAULT_RECORDS_PER_PAGE,
          offset: 0,
          q: query,
        }),
      );
    });
  });
});

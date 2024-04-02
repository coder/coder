import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { App } from "App";
import {
  MockEntitlementsWithAuditLog,
  MockMemberPermissions,
} from "testHelpers/entities";
import { server } from "testHelpers/server";
import { Language } from "./NavbarView";

/**
 * The LicenseBanner, mounted above the AppRouter, fetches entitlements. Thus, to test their
 * effects, we must test at the App level and `waitFor` the fetch to be done.
 */
describe("Navbar", () => {
  it("shows Audit Log link when permitted and entitled", async () => {
    // set entitlements to allow audit log
    server.use(
      http.get("/api/v2/entitlements", () => {
        return HttpResponse.json(MockEntitlementsWithAuditLog);
      }),
    );
    render(<App />);
    await waitFor(
      () => {
        const link = screen.getByText(Language.audit);
        expect(link).toBeDefined();
      },
      { timeout: 2000 },
    );
  });

  it("does not show Audit Log link when not entitled", async () => {
    // by default, user is an Admin with permission to see the audit log,
    // but is unlicensed so not entitled to see the audit log
    render(<App />);
    await waitFor(
      () => {
        const link = screen.queryByText(Language.audit);
        expect(link).toBe(null);
      },
      { timeout: 2000 },
    );
  });

  it("does not show Audit Log link when not permitted via role", async () => {
    // set permissions to Member (can't audit)
    server.use(
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json(MockMemberPermissions);
      }),
    );
    // set entitlements to allow audit log
    server.use(
      http.get("/api/v2/entitlements", () => {
        return HttpResponse.json(MockEntitlementsWithAuditLog);
      }),
    );
    render(<App />);
    await waitFor(
      () => {
        const link = screen.queryByText(Language.audit);
        expect(link).toBe(null);
      },
      { timeout: 2000 },
    );
  });
});

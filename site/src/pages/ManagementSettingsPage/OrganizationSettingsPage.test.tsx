import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import {
  MockDefaultOrganization,
  MockOrganization2,
} from "testHelpers/entities";
import {
  renderWithManagementSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import OrganizationSettingsPage from "./OrganizationSettingsPage";

jest.spyOn(console, "error").mockImplementation(() => {});

const renderRootPage = async () => {
  renderWithManagementSettingsLayout(<OrganizationSettingsPage />, {
    route: "/organizations",
    path: "/organizations",
    extraRoutes: [
      {
        path: "/organizations/:organization",
        element: <OrganizationSettingsPage />,
      },
    ],
  });
  await waitForLoaderToBeRemoved();
};

const renderPage = async (orgName: string) => {
  renderWithManagementSettingsLayout(<OrganizationSettingsPage />, {
    route: `/organizations/${orgName}`,
    path: "/organizations/:organization",
  });
  await waitForLoaderToBeRemoved();
};

describe("OrganizationSettingsPage", () => {
  it("has no organizations", async () => {
    server.use(
      http.get("/api/v2/organizations", () => {
        return HttpResponse.json([]);
      }),
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json({
          [`${MockDefaultOrganization.id}.editOrganization`]: true,
          viewDeploymentValues: true,
        });
      }),
    );
    await renderRootPage();
    await screen.findByText("No organizations found");
  });

  it("has no editable organizations", async () => {
    server.use(
      http.get("/api/v2/organizations", () => {
        return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
      }),
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json({
          viewDeploymentValues: true,
        });
      }),
    );
    await renderRootPage();
    await screen.findByText("No organizations found");
  });

  it("redirects to default organization", async () => {
    server.use(
      http.get("/api/v2/organizations", () => {
        // Default always preferred regardless of order.
        return HttpResponse.json([MockOrganization2, MockDefaultOrganization]);
      }),
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json({
          [`${MockDefaultOrganization.id}.editOrganization`]: true,
          [`${MockOrganization2.id}.editOrganization`]: true,
          viewDeploymentValues: true,
        });
      }),
    );
    await renderRootPage();
    const form = screen.getByTestId("org-settings-form");
    expect(within(form).getByRole("textbox", { name: "Name" })).toHaveValue(
      MockDefaultOrganization.name,
    );
  });

  it("redirects to non-default organization", async () => {
    server.use(
      http.get("/api/v2/organizations", () => {
        return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
      }),
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json({
          [`${MockOrganization2.id}.editOrganization`]: true,
          viewDeploymentValues: true,
        });
      }),
    );
    await renderRootPage();
    const form = screen.getByTestId("org-settings-form");
    expect(within(form).getByRole("textbox", { name: "Name" })).toHaveValue(
      MockOrganization2.name,
    );
  });

  it("cannot find organization", async () => {
    server.use(
      http.get("/api/v2/organizations", () => {
        return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
      }),
      http.post("/api/v2/authcheck", async () => {
        return HttpResponse.json({
          [`${MockOrganization2.id}.editOrganization`]: true,
          viewDeploymentValues: true,
        });
      }),
    );
    await renderPage("the-endless-void");
    await screen.findByText("Organization not found");
  });
});

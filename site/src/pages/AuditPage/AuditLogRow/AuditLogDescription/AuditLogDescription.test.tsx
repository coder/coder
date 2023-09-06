import {
  MockAuditLog,
  MockAuditLogWithWorkspaceBuild,
  MockWorkspaceCreateAuditLogForDifferentOwner,
  MockAuditLogSuccessfulLogin,
  MockAuditLogUnsuccessfulLoginKnownUser,
} from "testHelpers/entities";
import { AuditLogDescription } from "./AuditLogDescription";
import { AuditLogRow } from "../AuditLogRow";
import { render } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { i18n } from "i18n";

const t = (str: string, variables: Record<string, unknown>) =>
  i18n.t<string>(str, variables);

const getByTextContent = (text: string) => {
  return screen.getByText((_, element) => {
    const hasText = (element: Element | null) => element?.textContent === text;
    const elementHasText = hasText(element);
    const childrenDontHaveText = Array.from(element?.children || []).every(
      (child) => !hasText(child),
    );
    return elementHasText && childrenDontHaveText;
  });
};
describe("AuditLogDescription", () => {
  it("renders the correct string for a workspace create audit log", async () => {
    render(<AuditLogDescription auditLog={MockAuditLog} />);

    expect(screen.getByText("TestUser created workspace")).toBeDefined();
    expect(screen.getByText("bruno-dev")).toBeDefined();
  });

  it("renders the correct string for a workspace_build stop audit log", async () => {
    render(<AuditLogDescription auditLog={MockAuditLogWithWorkspaceBuild} />);

    expect(getByTextContent("TestUser stopped workspace test2")).toBeDefined();
  });

  it("renders the correct string for a workspace_build audit log with a duplicate word", async () => {
    const AuditLogWithRepeat = {
      ...MockAuditLogWithWorkspaceBuild,
      additional_fields: {
        workspace_name: "workspace",
      },
    };
    render(<AuditLogDescription auditLog={AuditLogWithRepeat} />);

    expect(
      getByTextContent("TestUser stopped workspace workspace"),
    ).toBeDefined();
  });
  it("renders the correct string for a workspace created for a different owner", async () => {
    render(
      <AuditLogDescription
        auditLog={MockWorkspaceCreateAuditLogForDifferentOwner}
      />,
    );

    expect(
      screen.getByText(
        `on behalf of ${MockWorkspaceCreateAuditLogForDifferentOwner.additional_fields.workspace_owner}`,
        { exact: false },
      ),
    ).toBeDefined();
  });
  it("renders the correct string for successful login", async () => {
    render(<AuditLogRow auditLog={MockAuditLogSuccessfulLogin} />);

    expect(
      screen.getByText(
        t("auditLog:table.logRow.description.unlinkedAuditDescription", {
          truncatedDescription: `${MockAuditLogSuccessfulLogin.user?.username} logged in`,
          target: "",
          onBehalfOf: undefined,
        })
          .replace(/<[^>]*>/g, " ")
          .replace(/\s{2,}/g, " ")
          .trim(),
      ),
    ).toBeInTheDocument();

    const statusPill = screen.getByRole("status");
    expect(statusPill).toHaveTextContent("201");
  });
  it("renders the correct string for unsuccessful login for a known user", async () => {
    render(<AuditLogRow auditLog={MockAuditLogUnsuccessfulLoginKnownUser} />);

    expect(
      screen.getByText(
        t("auditLog:table.logRow.description.unlinkedAuditDescription", {
          truncatedDescription: `${MockAuditLogUnsuccessfulLoginKnownUser.user?.username} logged in`,
          target: "",
          onBehalfOf: undefined,
        })
          .replace(/<[^>]*>/g, " ")
          .replace(/\s{2,}/g, " ")
          .trim(),
      ),
    ).toBeInTheDocument();

    const statusPill = screen.getByRole("status");
    expect(statusPill).toHaveTextContent("401");
  });
});

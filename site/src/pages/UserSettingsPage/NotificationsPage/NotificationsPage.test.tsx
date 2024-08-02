import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import type {
  Experiments,
  NotificationPreference,
  NotificationTemplate,
  UpdateUserNotificationPreferences,
} from "api/typesGenerated";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import NotificationsPage from "./NotificationsPage";

test("can enable and disable notifications", async () => {
  server.use(
    http.get("/api/v2/experiments", () =>
      HttpResponse.json(["notifications"] as Experiments),
    ),
    http.get("/api/v2/users/:userId/notifications/preferences", () =>
      HttpResponse.json(null),
    ),
    http.get("/api/v2/notifications/templates/system", () =>
      HttpResponse.json(notificationsTemplateSystemRes),
    ),
    http.put<
      { userId: string },
      UpdateUserNotificationPreferences,
      NotificationPreference[]
    >(
      "/api/v2/users/:userId/notifications/preferences",
      async ({ request }) => {
        const body = await request.json();
        const res: NotificationPreference[] = Object.entries(body).map(
          ([id, disabled]) => ({
            disabled,
            id,
            updated_at: new Date().toISOString(),
          }),
        );
        return HttpResponse.json(res);
      },
    ),
  );
  renderWithAuth(<NotificationsPage />);
  const user = userEvent.setup();
  const workspaceGroupTemplates = notificationsTemplateSystemRes.filter(
    (t) => t.group === "Workspace Events",
  );

  // Test notification groups
  const workspaceGroupSwitch = await screen.findByLabelText("Workspace Events");
  await user.click(workspaceGroupSwitch);
  await screen.findByText("Notification preferences updated");
  expect(workspaceGroupSwitch).not.toBeChecked();
  for (const template of workspaceGroupTemplates) {
    const templateSwitch = screen.getByLabelText(template.name);
    expect(templateSwitch).not.toBeChecked();
  }

  await user.click(workspaceGroupSwitch);
  await screen.findByText("Notification preferences updated");
  expect(workspaceGroupSwitch).toBeChecked();
  for (const template of workspaceGroupTemplates) {
    const templateSwitch = screen.getByLabelText(template.name);
    expect(templateSwitch).toBeChecked();
  }

  // Test individual notifications
  const workspaceDeletedSwitch = screen.getByLabelText("Workspace Deleted");
  await user.click(workspaceDeletedSwitch);
  await screen.findByText("Notification preferences updated");
  expect(workspaceDeletedSwitch).not.toBeChecked();

  await user.click(workspaceDeletedSwitch);
  await screen.findByText("Notification preferences updated");
  expect(workspaceDeletedSwitch).toBeChecked();
});

const notificationsTemplateSystemRes: NotificationTemplate[] = [
  {
    id: "f517da0b-cdc9-410f-ab89-a86107c420ed",
    name: "Workspace Deleted",
    title_template: 'Workspace "{{.Labels.name}}" deleted',
    body_template:
      'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** was deleted.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".',
    actions:
      '[{"url": "{{ base_url }}/workspaces", "label": "View workspaces"}, {"url": "{{ base_url }}/templates", "label": "View templates"}]',
    group: "Workspace Events",
    method: "",
    kind: "system",
  },
  {
    id: "381df2a9-c0c0-4749-420f-80a9280c66f9",
    name: "Workspace Autobuild Failed",
    title_template: 'Workspace "{{.Labels.name}}" autobuild failed',
    body_template:
      'Hi {{.UserName}}\nAutomatic build of your workspace **{{.Labels.name}}** failed.\nThe specified reason was "**{{.Labels.reason}}**".',
    actions:
      '[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
    group: "Workspace Events",
    method: "",
    kind: "system",
  },
  {
    id: "c34a0c09-0704-4cac-bd1c-0c0146811c2b",
    name: "Workspace updated automatically",
    title_template: 'Workspace "{{.Labels.name}}" updated automatically',
    body_template:
      "Hi {{.UserName}}\nYour workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).",
    actions:
      '[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
    group: "Workspace Events",
    method: "",
    kind: "system",
  },
  {
    id: "0ea69165-ec14-4314-91f1-69566ac3c5a0",
    name: "Workspace Marked as Dormant",
    title_template: 'Workspace "{{.Labels.name}}" marked as dormant',
    body_template:
      "Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.\nDormant workspaces are [automatically deleted](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) after {{.Labels.timeTilDormant}} of inactivity.\nTo prevent deletion, use your workspace with the link below.",
    actions:
      '[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
    group: "Workspace Events",
    method: "",
    kind: "system",
  },
  {
    id: "51ce2fdf-c9ca-4be1-8d70-628674f9bc42",
    name: "Workspace Marked for Deletion",
    title_template: 'Workspace "{{.Labels.name}}" marked for deletion',
    body_template:
      "Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.timeTilDormant}} of [dormancy](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) because of {{.Labels.reason}}.\nTo prevent deletion, use your workspace with the link below.",
    actions:
      '[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
    group: "Workspace Events",
    method: "",
    kind: "system",
  },
  {
    id: "4e19c0ac-94e1-4532-9515-d1801aa283b2",
    name: "User account created",
    title_template: 'User account "{{.Labels.created_account_name}}" created',
    body_template:
      "Hi {{.UserName}},\nNew user account **{{.Labels.created_account_name}}** has been created.",
    actions:
      '[{"url": "{{ base_url }}/deployment/users?filter=status%3Aactive", "label": "View accounts"}]',
    group: "User Events",
    method: "",
    kind: "system",
  },
];

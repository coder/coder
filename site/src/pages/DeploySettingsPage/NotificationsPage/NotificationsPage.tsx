import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Tooltip from "@mui/material/Tooltip";
import { Fragment, type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueries, useQueryClient } from "react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
  notificationDispatchMethods,
  selectTemplatesByGroup,
  systemNotificationTemplates,
  updateNotificationTemplateMethod,
} from "api/queries/notifications";
import type { NotificationsConfig } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import {
  castNotificationMethod,
  methodIcons,
  methodLabels,
  type NotificationMethod,
} from "modules/notifications/utils";
import { Section } from "pages/UserSettingsPage/Section";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import OptionsTable from "../OptionsTable";

type MethodToggleGroupProps = {
  templateId: string;
  options: NotificationMethod[];
  value: NotificationMethod;
};

const MethodToggleGroup: FC<MethodToggleGroupProps> = ({
  value,
  options,
  templateId,
}) => {
  const queryClient = useQueryClient();
  const updateMethodMutation = useMutation(
    updateNotificationTemplateMethod(templateId, queryClient),
  );

  return (
    <ToggleButtonGroup
      exclusive
      value={value}
      size="small"
      aria-label="Notification method"
      css={styles.toggleGroup}
      onChange={async (_, method) => {
        await updateMethodMutation.mutateAsync({
          method,
        });
        displaySuccess("Notification method updated");
      }}
    >
      {options.map((method) => {
        const Icon = methodIcons[method];
        const label = methodLabels[method];
        return (
          <Tooltip key={method} title={label}>
            <ToggleButton
              value={method}
              css={styles.toggleButton}
              onClick={(e) => {
                // Retain the value if the user clicks the same button, ensuring
                // at least one value remains selected.
                if (method === value) {
                  e.preventDefault();
                  e.stopPropagation();
                  return;
                }
              }}
            >
              <Icon aria-label={label} />
            </ToggleButton>
          </Tooltip>
        );
      })}
    </ToggleButtonGroup>
  );
};

export const NotificationsPage: FC = () => {
  const [searchParams] = useSearchParams();
  const { deploymentValues } = useDeploySettings();
  const [templatesByGroup, dispatchMethods] = useQueries({
    queries: [
      {
        ...systemNotificationTemplates(),
        select: selectTemplatesByGroup,
      },
      notificationDispatchMethods(),
    ],
  });
  const ready =
    templatesByGroup.data && dispatchMethods.data && deploymentValues;
  const tab = searchParams.get("tab") || "events";

  return (
    <>
      <Helmet>
        <title>{pageTitle("Notifications Settings")}</title>
      </Helmet>
      <Section
        title="Notifications"
        description={
          <>
            Control delivery methods for notifications on this deployment.
            Notifications may be disabled in your{" "}
            <Link
              to="/settings/notifications"
              css={(theme) => ({
                color: theme.roles.active.fill.outline,
                textDecoration: "none",
                "&: hover": {
                  textDecoration: "underline",
                },
              })}
            >
              profile settings
            </Link>
            .
          </>
        }
        layout="fluid"
      >
        <Tabs active={tab}>
          <TabsList>
            <TabLink to="?tab=events" value="events">
              Events
            </TabLink>
            <TabLink to="?tab=settings" value="settings">
              Settings
            </TabLink>
          </TabsList>
        </Tabs>

        <div css={styles.content}>
          {ready ? (
            tab === "events" ? (
              <EventsView
                defaultMethod={castNotificationMethod(
                  dispatchMethods.data.default,
                )}
                availableMethods={dispatchMethods.data.available.map(
                  castNotificationMethod,
                )}
                notificationsConfig={deploymentValues.config.notifications}
                templatesByGroup={templatesByGroup.data}
              />
            ) : (
              <OptionsTable
                options={deploymentValues?.options.filter((o) =>
                  deploymentGroupHasParent(o.group, "Notifications"),
                )}
              />
            )
          ) : (
            <Loader />
          )}
        </div>
      </Section>
    </>
  );
};

type EventsViewProps = {
  defaultMethod: NotificationMethod;
  availableMethods: NotificationMethod[];
  notificationsConfig?: NotificationsConfig;
  templatesByGroup: ReturnType<typeof selectTemplatesByGroup>;
};

const EventsView: FC<EventsViewProps> = ({
  defaultMethod,
  availableMethods,
  notificationsConfig,
  templatesByGroup,
}) => {
  const isUsingWebhook = availableMethods.includes("webhook");
  const isUsingSmpt = availableMethods.includes("smtp");
  const webhookEndpoint = notificationsConfig?.webhook.endpoint;
  const smtpConfig = notificationsConfig?.email;

  return (
    <Stack spacing={3}>
      {isUsingWebhook && !webhookEndpoint && (
        <Alert severity="warning">
          Webhook method is enabled, but the endpoint is not configured.
        </Alert>
      )}

      {isUsingSmpt && !smtpConfig && (
        <Alert severity="warning">
          SMTP method is enabled, but is not configured.
        </Alert>
      )}

      {Object.entries(templatesByGroup).map(([group, templates]) => (
        <Card
          key={group}
          variant="outlined"
          css={{ background: "transparent", width: "100%" }}
        >
          <List>
            <ListItem css={styles.listHeader}>
              <ListItemText css={styles.listItemText} primary={group} />
            </ListItem>

            {templates.map((tpl) => {
              const value = castNotificationMethod(tpl.method || defaultMethod);

              return (
                <Fragment key={tpl.id}>
                  <ListItem>
                    <ListItemText
                      css={styles.listItemText}
                      primary={tpl.name}
                    />
                    <MethodToggleGroup
                      templateId={tpl.id}
                      options={availableMethods}
                      value={value}
                    />
                  </ListItem>
                  <Divider css={styles.divider} />
                </Fragment>
              );
            })}
          </List>
        </Card>
      ))}
    </Stack>
  );
};

export default NotificationsPage;

const styles = {
  content: { paddingTop: 24 },
  listHeader: (theme) => ({
    background: theme.palette.background.paper,
    borderBottom: `1px solid ${theme.palette.divider}`,
  }),
  listItemText: {
    [`& .${listItemTextClasses.primary}`]: {
      fontSize: 14,
      fontWeight: 500,
    },
    [`& .${listItemTextClasses.secondary}`]: {
      fontSize: 14,
    },
  },
  toggleGroup: (theme) => ({
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,
  }),
  toggleButton: (theme) => ({
    border: 0,
    borderRadius: 4,
    fontSize: 16,
    padding: "4px 8px",
    color: theme.palette.text.disabled,

    "&:hover": {
      color: theme.palette.text.primary,
    },

    "& svg": {
      fontSize: "inherit",
    },
  }),
  divider: {
    "&:last-child": {
      display: "none",
    },
  },
} as Record<string, Interpolation<Theme>>;

import type { Interpolation, Theme } from "@emotion/react";
import AlertTitle from "@mui/material/AlertTitle";
import Button from "@mui/material/Button";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Tooltip from "@mui/material/Tooltip";
import { Fragment, type FC } from "react";
import { useMutation, useQueries, useQueryClient } from "react-query";
import {
  notificationDispatchMethods,
  selectTemplatesByGroup,
  systemNotificationTemplates,
  updateNotificationTemplateMethod,
} from "api/queries/notifications";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useClipboard } from "hooks";
import {
  castNotificationMethod,
  methodIcons,
  methodLabels,
  type NotificationMethod,
} from "modules/notifications/utils";
import { Section } from "pages/UserSettingsPage/Section";
import { useDeploySettings } from "../DeploySettingsLayout";

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
  const ready = templatesByGroup.data && dispatchMethods.data;

  const isUsingWebhook = dispatchMethods.data?.available.includes("webhook");
  const webhookEndpoint =
    deploymentValues?.config.notifications?.webhook.endpoint;

  return (
    <Section
      title="Notifications"
      description="Control delivery methods for notifications. Settings applied to this deployment."
      layout="fluid"
    >
      {ready ? (
        <Stack spacing={3}>
          {isUsingWebhook &&
            (webhookEndpoint ? (
              <WebhookInfo endpoint={webhookEndpoint} />
            ) : (
              <Alert severity="warning">
                Webhook method is enabled, but the endpoint is not configured.
              </Alert>
            ))}
          {Object.entries(templatesByGroup.data).map(([group, templates]) => (
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
                  const value = castNotificationMethod(
                    tpl.method || dispatchMethods.data.default,
                  );
                  const options = dispatchMethods.data.available.map(
                    castNotificationMethod,
                  );

                  return (
                    <Fragment key={tpl.id}>
                      <ListItem>
                        <ListItemText
                          css={styles.listItemText}
                          primary={tpl.name}
                        />
                        <MethodToggleGroup
                          templateId={tpl.id}
                          options={options}
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
      ) : (
        <Loader />
      )}
    </Section>
  );
};

export default NotificationsPage;

type WebhookInfoProps = {
  endpoint: string;
};

const WebhookInfo = ({ endpoint }: WebhookInfoProps) => {
  const clipboard = useClipboard({ textToCopy: endpoint });

  return (
    <Alert
      severity="info"
      actions={
        <Button
          variant="text"
          onClick={clipboard.copyToClipboard}
          disabled={clipboard.showCopiedSuccess}
        >
          {clipboard.showCopiedSuccess ? "Copied!" : "Copy"}
        </Button>
      }
    >
      <AlertTitle>Webhook Endpoint</AlertTitle>
      <AlertDetail>{endpoint}</AlertDetail>
    </Alert>
  );
};

const styles = {
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

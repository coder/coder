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
import { useMutation, useQueries } from "react-query";
import {
  notificationDispatchMethods,
  selectTemplatesByGroup,
  systemNotificationTemplatesByGroup,
  updateNotificationTemplateMethod,
} from "api/queries/notifications";
import type { NotificationTemplateMethod } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { methodIcons, methodLabel } from "modules/notifications/utils";
import { Section } from "pages/UserSettingsPage/Section";
import { useDeploySettings } from "../DeploySettingsLayout";

type MethodToggleGroupProps = {
  templateId: string;
  value: string;
  available: readonly NotificationTemplateMethod[];
  defaultMethod: NotificationTemplateMethod;
};

const MethodToggleGroup: FC<MethodToggleGroupProps> = ({
  value,
  available,
  templateId,
  defaultMethod,
}) => {
  const updateMethodMutation = useMutation(
    updateNotificationTemplateMethod(templateId),
  );
  const options: NotificationTemplateMethod[] = ["", ...available];

  return (
    <ToggleButtonGroup
      exclusive
      value={value}
      size="small"
      aria-label="Notification method"
      css={styles.toggleGroup}
      onChange={async (_, method) => {
        // Retain the value if the user clicks the same button, ensuring
        // at least one value remains selected.
        if (method === value) {
          return;
        }

        await updateMethodMutation.mutateAsync({
          method,
        });
        displaySuccess("Notification method updated");
      }}
    >
      {options.map((method) => {
        const Icon = methodIcons[method];
        const label = methodLabel(method, defaultMethod);
        return (
          <Tooltip key={method} title={label}>
            <ToggleButton value={method} css={styles.toggleButton}>
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
        ...systemNotificationTemplatesByGroup(),
        select: selectTemplatesByGroup,
      },
      notificationDispatchMethods(),
    ],
  });
  const ready = templatesByGroup.data && dispatchMethods.data;

  const shouldDisplayWebhookWarning =
    deploymentValues.config.notifications?.webhook.endpoint === "" &&
    dispatchMethods.data?.available.includes("webhook");

  return (
    <Section
      title="Notification Targets"
      description="Control delivery methods for notifications. Settings applied to this deployment."
      layout="fluid"
    >
      {ready ? (
        <Stack spacing={3}>
          {shouldDisplayWebhookWarning && (
            <Alert severity="warning">
              Webhook method is enabled, but the endpoint is not configured.
            </Alert>
          )}
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
                  return (
                    <Fragment key={tpl.id}>
                      <ListItem>
                        <ListItemText
                          css={styles.listItemText}
                          primary={tpl.name}
                        />
                        <MethodToggleGroup
                          defaultMethod={dispatchMethods.data.default}
                          templateId={tpl.id}
                          available={dispatchMethods.data.available}
                          value={tpl.method}
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

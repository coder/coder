import type { Interpolation, Theme } from "@emotion/react";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import Switch from "@mui/material/Switch";
import Tooltip from "@mui/material/Tooltip";
import { Fragment, type FC } from "react";
import { useMutation, useQueries, useQueryClient } from "react-query";
import {
  notificationDispatchMethods,
  selectTemplatesByGroup,
  systemNotificationTemplatesByGroup,
  updateUserNotificationPreferences,
  userNotificationPreferences,
} from "api/queries/notifications";
import type { NotificationPreference } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { methodIcons, methodLabel } from "modules/notifications/utils";
import { Section } from "../Section";

type PreferenceSwitchProps = {
  id: string;
  disabled: boolean;
  onToggle: (checked: boolean) => Record<string, boolean>;
};

const PreferenceSwitch: FC<PreferenceSwitchProps> = ({
  id,
  disabled,
  onToggle,
}) => {
  const { user } = useAuthenticated();
  const queryClient = useQueryClient();
  const updatePreferences = useMutation(
    updateUserNotificationPreferences(user.id, queryClient),
  );

  return (
    <Switch
      id={id}
      size="small"
      checked={!disabled}
      onChange={async (_, checked) => {
        await updatePreferences.mutateAsync({
          template_disabled_map: onToggle(checked),
        });
        displaySuccess("Notification preferences updated");
      }}
    />
  );
};

export const NotificationsPage: FC = () => {
  const { user } = useAuthenticated();
  const [disabledPreferences, templatesByGroup, dispatchMethods] = useQueries({
    queries: [
      {
        ...userNotificationPreferences(user.id),
        select: selectDisabledPreferences,
      },
      {
        ...systemNotificationTemplatesByGroup(),
        select: selectTemplatesByGroup,
      },
      notificationDispatchMethods(),
    ],
  });
  const ready =
    disabledPreferences.data && templatesByGroup.data && dispatchMethods.data;

  return (
    <Section
      title="Notifications"
      description="Configure notifications. Some may be disabled by the deployment administrator."
      layout="fluid"
    >
      {ready ? (
        <Stack spacing={3}>
          {Object.entries(templatesByGroup.data).map(([group, templates]) => {
            const allDisabled = templates.some((tpl) => {
              return disabledPreferences.data[tpl.id] === true;
            });

            return (
              <Card
                variant="outlined"
                css={{ background: "transparent" }}
                key={group}
              >
                <List>
                  <ListItem css={styles.listHeader}>
                    <ListItemIcon>
                      <PreferenceSwitch
                        id={group}
                        disabled={allDisabled}
                        onToggle={(checked) => {
                          const updated = { ...disabledPreferences.data };
                          for (const tpl of templates) {
                            updated[tpl.id] = !checked;
                          }
                          return updated;
                        }}
                      />
                    </ListItemIcon>
                    <ListItemText
                      css={styles.listItemText}
                      primary={group}
                      primaryTypographyProps={{
                        component: "label",
                        htmlFor: group,
                      }}
                    />
                  </ListItem>
                  {templates.map((tmpl) => {
                    const Icon = methodIcons[tmpl.method];
                    const label = methodLabel(
                      tmpl.method,
                      dispatchMethods.data.default,
                    );

                    return (
                      <Fragment key={tmpl.id}>
                        <ListItem>
                          <ListItemIcon>
                            <PreferenceSwitch
                              id={tmpl.id}
                              disabled={disabledPreferences.data[tmpl.id]}
                              onToggle={(checked) => {
                                return {
                                  ...disabledPreferences.data,
                                  [tmpl.id]: !checked,
                                };
                              }}
                            />
                          </ListItemIcon>
                          <ListItemText
                            primaryTypographyProps={{
                              component: "label",
                              htmlFor: tmpl.id,
                            }}
                            css={styles.listItemText}
                            primary={tmpl.name}
                          />
                          <ListItemIcon
                            css={styles.listItemEndIcon}
                            aria-label="Delivery method"
                          >
                            <Tooltip title={label}>
                              <Icon aria-label={label} />
                            </Tooltip>
                          </ListItemIcon>
                        </ListItem>
                        <Divider css={styles.divider} />
                      </Fragment>
                    );
                  })}
                </List>
              </Card>
            );
          })}
        </Stack>
      ) : (
        <Loader />
      )}
    </Section>
  );
};

export default NotificationsPage;

function selectDisabledPreferences(data: NotificationPreference[]) {
  return data.reduce(
    (acc, pref) => {
      acc[pref.id] = pref.disabled;
      return acc;
    },
    {} as Record<string, boolean>,
  );
}

const styles = {
  listHeader: (theme) => ({
    background: theme.palette.background.paper,
    borderBottom: `1px solid ${theme.palette.divider}`,
  }),
  listItemText: {
    [`& .${listItemTextClasses.primary}`]: {
      fontSize: 14,
      fontWeight: 500,
      textTransform: "capitalize",
    },
    [`& .${listItemTextClasses.secondary}`]: {
      fontSize: 14,
    },
  },
  listItemEndIcon: (theme) => ({
    minWidth: 0,
    fontSize: 20,
    color: theme.palette.text.secondary,

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

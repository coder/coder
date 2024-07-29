import type { Interpolation, Theme } from "@emotion/react";
import EmailIcon from "@mui/icons-material/EmailOutlined";
import WebhookIcon from "@mui/icons-material/LanguageOutlined";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import type { FC } from "react";
import { FormFields, FormSection, HorizontalForm } from "components/Form/Form";
import { Section } from "pages/UserSettingsPage/Section";
import Button from "@mui/material/Button";
import { Stack } from "components/Stack/Stack";

export const NotificationsPage: FC = () => {
  return (
    <Section
      title="Notification Targets"
      description="Control delivery methods for notifications. Settings applied to this deployment."
      layout="fluid"
    >
      <HorizontalForm>
        <FormSection
          title="Webhook Target"
          description="A webhook target is a URL that you can use to receive your events through an API."
        >
          <FormFields>
            <TextField
              label="Webhook Target URL"
              placeholder="https://myapi.com/events"
              helperText="Leave this empty to disable webhook notifications."
            />
            <Stack direction="row" spacing={1}>
              <Button>Reset</Button>
              <Button variant="contained" color="primary">
                Save URL
              </Button>
            </Stack>
          </FormFields>
        </FormSection>

        <FormSection
          title="Events"
          description="Update the events to use the correct targets for each notification."
        >
          <FormFields>
            <Card
              variant="outlined"
              css={{ background: "transparent", width: "100%" }}
            >
              <List>
                <ListItem css={styles.listHeader}>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="User events"
                  />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="User added"
                  />
                  <ToggleButtonGroup
                    value="email"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="User removed"
                  />
                  <ToggleButtonGroup
                    value="email"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="User suspended"
                  />
                  <ToggleButtonGroup
                    value="webhook"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
              </List>
            </Card>

            <Card
              variant="outlined"
              css={{ background: "transparent", width: "100%" }}
            >
              <List>
                <ListItem css={styles.listHeader}>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="Workspace events"
                  />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="Dormancy"
                    secondary="When a workspace is marked as dormant"
                  />
                  <ToggleButtonGroup
                    value="email"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="Deletion"
                    secondary="When a workspace is marked for deletion"
                  />
                  <ToggleButtonGroup
                    value="email"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
                <Divider />
                <ListItem>
                  <ListItemIcon>
                    <Switch size="small" />
                  </ListItemIcon>
                  <ListItemText
                    css={styles.listItemText}
                    primary="Build failure"
                    secondary="When a workspace fails to build"
                  />
                  <ToggleButtonGroup
                    value="webhook"
                    size="small"
                    aria-label="Targe"
                    css={styles.toggleGroup}
                  >
                    <ToggleButton
                      value="email"
                      key="email"
                      css={styles.toggleButton}
                    >
                      <EmailIcon />
                    </ToggleButton>
                    <ToggleButton
                      value="webhook"
                      key="webhook"
                      css={styles.toggleButton}
                    >
                      <WebhookIcon />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </ListItem>
              </List>
            </Card>
          </FormFields>
        </FormSection>
      </HorizontalForm>
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
} as Record<string, Interpolation<Theme>>;

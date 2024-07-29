import type { Interpolation, Theme } from "@emotion/react";
import EmailIcon from "@mui/icons-material/EmailOutlined";
import Card from "@mui/material/Card";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText, { listItemTextClasses } from "@mui/material/ListItemText";
import Switch from "@mui/material/Switch";
import type { FC } from "react";
import { Section } from "../Section";

export const NotificationsPage: FC = () => {
  return (
    <Section
      title="Notifications"
      description="Configure notifications. Some may be disabled by the deployment administrator."
      layout="fluid"
    >
      <Card variant="outlined" css={{ background: "transparent" }}>
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
            <ListItemIcon css={styles.listItemEndIcon}>
              <EmailIcon />
            </ListItemIcon>
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
            <ListItemIcon css={styles.listItemEndIcon}>
              <EmailIcon />
            </ListItemIcon>
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
            <ListItemIcon css={styles.listItemEndIcon}>
              <EmailIcon />
            </ListItemIcon>
          </ListItem>
        </List>
      </Card>
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
  listItemEndIcon: (theme) => ({
    minWidth: 0,
    fontSize: 20,
    color: theme.palette.text.secondary,

    "& svg": {
      fontSize: "inherit",
    },
  }),
} as Record<string, Interpolation<Theme>>;

import { makeStyles } from "@mui/styles";
import { AppPreviewLink } from "components/Resources/AppLink/AppPreviewLink";
import { Maybe } from "components/Conditionals/Maybe";
import { FC } from "react";
import { useTranslation } from "react-i18next";
import { combineClasses } from "utils/combineClasses";
import { WorkspaceAgent } from "../../api/typesGenerated";
import { Stack } from "../Stack/Stack";

interface AgentRowPreviewStyles {
  // Helpful when there are more than one row so the values are aligned
  // When it is only one row, it is better to have than "flex" and not hard aligned
  alignValues?: boolean;
}
export interface AgentRowPreviewProps extends AgentRowPreviewStyles {
  agent: WorkspaceAgent;
}

export const AgentRowPreview: FC<AgentRowPreviewProps> = ({
  agent,
  alignValues,
}) => {
  const styles = useStyles({ alignValues });
  const { t } = useTranslation("agent");

  return (
    <Stack
      key={agent.id}
      direction="row"
      alignItems="center"
      justifyContent="space-between"
      className={styles.agentRow}
    >
      <Stack direction="row" alignItems="baseline">
        <div className={styles.agentStatusWrapper}>
          <div className={styles.agentStatusPreview}></div>
        </div>
        <Stack
          alignItems="baseline"
          direction="row"
          spacing={4}
          className={styles.agentData}
        >
          <Stack
            direction="row"
            alignItems="baseline"
            spacing={1}
            className={combineClasses([
              styles.noShrink,
              styles.agentDataItem,
              styles.agentDataName,
            ])}
          >
            <span>{t("labels.agent").toString()}:</span>
            <span className={styles.agentDataValue}>{agent.name}</span>
          </Stack>

          <Stack
            direction="row"
            alignItems="baseline"
            spacing={1}
            className={combineClasses([
              styles.noShrink,
              styles.agentDataItem,
              styles.agentDataOS,
            ])}
          >
            <span>{t("labels.os").toString()}:</span>
            <span
              className={combineClasses([
                styles.agentDataValue,
                styles.agentOS,
              ])}
            >
              {agent.operating_system}
            </span>
          </Stack>

          <Stack
            direction="row"
            alignItems="center"
            spacing={1}
            className={styles.agentDataItem}
          >
            <span>{t("labels.apps").toString()}:</span>
            <Stack
              direction="row"
              alignItems="center"
              spacing={0.5}
              wrap="wrap"
            >
              {agent.apps.map((app) => (
                <AppPreviewLink key={app.slug} app={app} />
              ))}
              <Maybe condition={agent.apps.length === 0}>
                <span className={styles.agentDataValue}>
                  {t("labels.noApps")}
                </span>
              </Maybe>
            </Stack>
          </Stack>
        </Stack>
      </Stack>
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  agentRow: {
    padding: theme.spacing(2, 4),
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,
    position: "relative",

    "&:not(:last-child)": {
      paddingBottom: 0,
    },

    "&:after": {
      content: "''",
      height: "100%",
      width: 2,
      backgroundColor: theme.palette.divider,
      position: "absolute",
      top: 0,
      left: 49,
    },
  },

  agentStatusWrapper: {
    width: theme.spacing(4.5),
    display: "flex",
    justifyContent: "center",
    flexShrink: 0,
  },

  agentStatusPreview: {
    width: 10,
    height: 10,
    border: `2px solid ${theme.palette.text.secondary}`,
    borderRadius: "100%",
    position: "relative",
    zIndex: 1,
    background: theme.palette.background.paper,
  },

  agentName: {
    fontWeight: 600,
  },

  agentOS: {
    textTransform: "capitalize",
    fontSize: 14,
    color: theme.palette.text.secondary,
  },

  agentData: {
    fontSize: 14,
    color: theme.palette.text.secondary,

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(2),
      flexWrap: "wrap",
    },
  },

  agentDataName: {
    [theme.breakpoints.up("sm")]: {
      minWidth: ({ alignValues }: AgentRowPreviewStyles) =>
        alignValues ? 240 : undefined,
    },
  },

  agentDataOS: {
    [theme.breakpoints.up("sm")]: {
      minWidth: ({ alignValues }: AgentRowPreviewStyles) =>
        alignValues ? 100 : undefined,
    },
  },

  agentDataValue: {
    color: theme.palette.text.primary,
  },

  noShrink: {
    flexShrink: 0,
  },

  agentDataItem: {
    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
      alignItems: "flex-start",
      gap: theme.spacing(1),
      width: "fit-content",
    },
  },
}));

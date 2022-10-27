import { makeStyles } from "@material-ui/core/styles"
import { AppPreviewLink } from "components/AppLink/AppPreviewLink"
import { FC } from "react"
import { combineClasses } from "util/combineClasses"
import { WorkspaceAgent } from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"

export interface AgentRowPreviewProps {
  agent: WorkspaceAgent
}

export const AgentRowPreview: FC<AgentRowPreviewProps> = ({ agent }) => {
  const styles = useStyles()

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
          <Stack direction="row" alignItems="baseline" spacing={1}>
            <span>Agent:</span>
            <span className={styles.agentDataValue}>{agent.name}</span>
          </Stack>

          <Stack direction="row" alignItems="baseline" spacing={1}>
            <span>OS:</span>
            <span
              className={combineClasses([
                styles.agentDataValue,
                styles.agentOS,
              ])}
            >
              {agent.operating_system}
            </span>
          </Stack>

          <Stack direction="row" alignItems="baseline" spacing={1}>
            <span>Apps:</span>
            <Stack
              direction="row"
              alignItems="center"
              spacing={0.5}
              wrap="wrap"
            >
              {agent.apps.map((app) => (
                <AppPreviewLink key={app.name} app={app} />
              ))}
            </Stack>
          </Stack>
        </Stack>
      </Stack>
    </Stack>
  )
}

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
  },

  agentDataValue: {
    color: theme.palette.text.primary,
  },
}))

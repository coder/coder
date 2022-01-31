import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import OpenInNewIcon from "@material-ui/icons/OpenInNew"
import React from "react"

import * as API from "../../api"

export interface WorkspaceProps {
  workspace: API.Workspace
}

const useStatusStyles = makeStyles((theme) => {
  const common = {
    width: theme.spacing(1),
    height: theme.spacing(1),
    borderRadius: "100%",
    backgroundColor: theme.palette.action.disabled,
    transition: "background-color 200ms ease",
  }

  return {
    inactive: common,
    active: {
      ...common,
      backgroundColor: theme.palette.primary.main,
    },
  }
})

/**
 * A component that displays the Dev URL indicator. The indicator status represents
 * loading, online, offline, or error.
 */
export const StatusIndicator: React.FC<{ status: ResourceStatus }> = ({ status }) => {
  const styles = useStatusStyles()

  if (status == "loading") {
    return <CircularProgress width={"16px"} />
  } else {
    const className = status === "active" ? styles.active : styles.inactive
    return <div className={className} />
  }
}

type ResourceStatus = "active" | "inactive" | "loading"

export interface ResourceRowProps {
  name: string
  icon: string
  //href: string
  status: ResourceStatus
}

const ResourceIconSize = 20

export const ResourceRow: React.FC<ResourceRowProps> = ({ icon, href, name, status }) => {
  const styles = useResourceRowStyles()

  return (
    <div className={styles.root}>
      <div className={styles.iconContainer}>
        <img src={icon} height={ResourceIconSize} width={ResourceIconSize} />
      </div>
      <div className={styles.nameContainer}>
        <Link
          color={status === "active" ? "primary" : "initial"}
          href={href}
          rel="noreferrer noopener"
          target="_blank"
          underline="none"
        >
          <span>{name}</span>
          <OpenInNewIcon fontSize="inherit" style={{ marginTop: "0.25em", marginLeft: "0.5em" }} />
        </Link>
      </div>
      <div className={styles.statusContainer}>
        <StatusIndicator status={status} />
      </div>
    </div>
  )
}

const useResourceRowStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  iconContainer: {
    width: ResourceIconSize + theme.spacing(1),
    height: ResourceIconSize + theme.spacing(1),
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
    flex: 0,
  },
  nameContainer: {
    margin: theme.spacing(1),
    paddingLeft: theme.spacing(1),
    flex: 1,
    width: "100%",
  },
  statusContainer: {
    width: 24,
    height: 24,
    flex: 0,
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
}))

export const Title: React.FC = ({ children }) => {
  const styles = useTitleStyles()

  return <div className={styles.header}>{children}</div>
}

const useTitleStyles = makeStyles((theme) => ({
  header: {
    alignItems: "center",
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    flexDirection: "row",
    height: theme.spacing(6),
    marginBottom: theme.spacing(2),
    marginTop: -theme.spacing(1),
    paddingBottom: theme.spacing(1),
    paddingLeft: Constants.CardPadding + theme.spacing(1),
    paddingRight: Constants.CardPadding / 2,
  },
}))

import Timeline from "@material-ui/lab/Timeline"
import TimelineItem from "@material-ui/lab/TimelineItem"
import TimelineSeparator from "@material-ui/lab/TimelineSeparator"
import TimelineConnector from "@material-ui/lab/TimelineConnector"
import TimelineContent from "@material-ui/lab/TimelineContent"
import TimelineDot from "@material-ui/lab/TimelineDot"
import { QuestionHelp } from "../QuestionHelp"
import { CircularProgress, Link } from "@material-ui/core"

export const WorkspaceTimeline: React.FC = () => {
  return (
    <Timeline>
      <TimelineItem>
        <TimelineSeparator>
          <TimelineDot />
          <TimelineConnector />
        </TimelineSeparator>
        <TimelineContent>Eat</TimelineContent>
      </TimelineItem>
      <TimelineItem>
        <TimelineSeparator>
          <TimelineDot />
          <TimelineConnector />
        </TimelineSeparator>
        <TimelineContent>Code</TimelineContent>
      </TimelineItem>
      <TimelineItem>
        <TimelineSeparator>
          <TimelineDot />
        </TimelineSeparator>
        <TimelineContent>Sleep</TimelineContent>
      </TimelineItem>
    </Timeline>
  )
}

export const Workspace: React.FC<WorkspaceProps> = ({ workspace }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <Paper elevation={0} className={styles.section}>
        <Typography variant="h4">{workspace.name}</Typography>
        <Typography variant="body2" color="textSecondary">
          {"TODO: Project"}
        </Typography>
      </Paper>
      <div className={styles.horizontal}>
        <div className={styles.sideBar}>
          <Paper elevation={0} className={styles.section}>
            <Title>
              <Typography variant="h6">Applications</Typography>
              <div style={{ margin: "0em 1em" }}>
                <QuestionHelp />
              </div>
            </Title>

            <div className={styles.vertical}>
              <ResourceRow name={"Code Web"} icon={"/static/vscode.svg"} status={"active"} />
              <ResourceRow name={"Terminal"} icon={"/static/terminal.svg"} status={"active"} />
              <ResourceRow name={"React App"} icon={"/static/react-icon.svg"} status={"loading"} />
            </div>
          </Paper>
          <Paper elevation={0} className={styles.section}>
            <Title>
              <Typography variant="h6">Resources</Typography>
              <div style={{ margin: "0em 1em" }}>
                <QuestionHelp />
              </div>
            </Title>

            <div className={styles.vertical}>
              <ResourceRow name={"GCS Bucket"} icon={"/static/google-storage-logo.svg"} status={"active"} />
              <ResourceRow name={"Windows (x64 - VM)"} icon={"/static/windows-logo.svg"} status={"active"} />
              <ResourceRow name={"OSX (M1 - Physical)"} icon={"/static/apple-logo.svg"} status={"inactive"} />
            </div>
          </Paper>
        </div>
        <Paper elevation={0} className={styles.main}>
          <Title>
            <Typography variant="h6">Timeline</Typography>
          </Title>
          <WorkspaceTimeline />
        </Paper>
      </div>
    </div>
  )
}

namespace Constants {
  export const CardRadius = 8
  export const CardPadding = 20
}

export const useStyles = makeStyles((theme) => {
  const common = {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: Constants.CardRadius,
    margin: theme.spacing(1),
    padding: Constants.CardPadding,
  }

  return {
    root: {
      display: "flex",
      flexDirection: "column",
    },
    horizontal: {
      display: "flex",
      flexDirection: "row",
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
    section: common,
    sideBar: {
      display: "flex",
      flexDirection: "column",
      width: "400px",
    },
    main: {
      ...common,
      flex: 1,
    },
  }
})

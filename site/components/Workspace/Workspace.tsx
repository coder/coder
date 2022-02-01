import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import OpenInNewIcon from "@material-ui/icons/OpenInNew"
import React, { useState } from "react"
import MoreVertIcon from "@material-ui/icons/MoreVert"
import { QuestionHelp } from "../QuestionHelp"
import { CircularProgress, IconButton, Link, Menu, MenuItem } from "@material-ui/core"

import { Timeline as TestTimeline } from "../Timeline"

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
    return <CircularProgress style={{ width: "12px", height: "12px" }} />
  } else {
    const className = status === "active" ? styles.active : styles.inactive
    return <div className={className} />
  }
}

type ResourceStatus = "active" | "inactive" | "loading"

export interface ResourceRowProps {
  name: string
  icon: string
  href?: string
  status: ResourceStatus
}

const ResourceIconSize = 20

export const ResourceRow: React.FC<ResourceRowProps> = ({ icon, href, name, status }) => {
  const styles = useResourceRowStyles()

  const [menuAnchorEl, setMenuAnchorEl] = useState<HTMLElement | null>(null)

  return (
    <div className={styles.root}>
      <div className={styles.iconContainer}>
        <img src={icon} height={ResourceIconSize} width={ResourceIconSize} />
      </div>
      <div className={styles.nameContainer}>
        {href ? (
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
        ) : (
          <span>{name}</span>
        )}
      </div>
      <div className={styles.statusContainer}>
        <StatusIndicator status={status} />
      </div>
      <div>
        <IconButton size="small" className={styles.action} onClick={(ev) => setMenuAnchorEl(ev.currentTarget)}>
          <MoreVertIcon fontSize="inherit" />
        </IconButton>

        <Menu anchorEl={menuAnchorEl} open={!!menuAnchorEl} onClose={() => setMenuAnchorEl(undefined)}>
          <MenuItem
            onClick={() => {
              setMenuAnchorEl(undefined)
            }}
          >
            SSH
          </MenuItem>
          <MenuItem
            onClick={() => {
              setMenuAnchorEl(undefined)
            }}
          >
            Remote Desktop
          </MenuItem>
        </Menu>
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
  action: {
    margin: `0 ${theme.spacing(0.5)}px`,
    opacity: 0.7,
    fontSize: 16,
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

const TitleIconSize = 48

export const Workspace: React.FC<WorkspaceProps> = ({ workspace }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <Paper elevation={0} className={styles.section}>
        <div className={styles.horizontal}>
          <Box mr={"1em"}>
            <img src={"/static/discord-logo.svg"} height={TitleIconSize} width={TitleIconSize} />
          </Box>
          <div className={styles.vertical}>
            <Typography variant="h4">{workspace.name}</Typography>
            <Typography variant="body2" color="textSecondary">
              <Link>test-org</Link>
              {" / "}
              <Link>test-project</Link>
            </Typography>
          </div>
        </div>
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
              <ResourceRow name={"Code Web"} icon={"/static/vscode.svg"} href={"placeholder"} status={"active"} />
              <ResourceRow name={"Terminal"} icon={"/static/terminal.svg"} href={"placeholder"} status={"active"} />
              <ResourceRow name={"React App"} icon={"/static/react-icon.svg"} href={"placeholder"} status={"active"} />
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
              <ResourceRow
                name={"GCS Bucket"}
                icon={"/static/google-storage-logo.svg"}
                href={"placeholder"}
                status={"active"}
              />
              <ResourceRow name={"Windows (x64 - VM)"} icon={"/static/windows-logo.svg"} status={"active"} />
              <ResourceRow name={"OSX (M1 - Physical)"} icon={"/static/apple-logo.svg"} status={"inactive"} />
            </div>
          </Paper>
        </div>
        <Paper elevation={0} className={styles.main}>
          <Title>
            <Typography variant="h6">Timeline</Typography>
          </Title>
          <TestTimeline />
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

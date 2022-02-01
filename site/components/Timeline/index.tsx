import { Avatar, Box, SvgIcon, Typography } from "@material-ui/core"
import makeStyles from "@material-ui/styles/makeStyles";
import React, { useState } from "react"
import { TerminalOutput } from "./TerminalOutput";

export interface TimelineEntry {
  date: Date
  title: string
  description?: string
}

const today = new Date();
const yesterday = new Date()
yesterday.setHours(-24)
const weekAgo = new Date()
weekAgo.setHours(-24 * 7)

const sampleOutput = `
Successfully assigned coder/bryan-prototype-jppnd to gke-master-workspaces-1-ef039342-cybd
Container image "gke.gcr.io/istio/proxyv2:1.4.10-gke.8" already present on machine
Created container istio-init
Started container istio-init
Pulling image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131"
Successfully pulled image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131" in 7.423772294s
Successfully assigned coder/bryan-prototype-jppnd to gke-master-workspaces-1-ef039342-cybd
Container image "gke.gcr.io/istio/proxyv2:1.4.10-gke.8" already present on machine
Created container istio-init
Started container istio-init
Pulling image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131"
Successfully pulled image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131" in 7.423772294s
Successfully assigned coder/bryan-prototype-jppnd to gke-master-workspaces-1-ef039342-cybd
Container image "gke.gcr.io/istio/proxyv2:1.4.10-gke.8" already present on machine
Created container istio-init
Started container istio-init
Pulling image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131"
Successfully pulled image "gcr.io/coder-enterprise-nightlies/coder/envbox:1.27.0-rc.0-145-g8d4ee2e9e-20220131" in 7.423772294s
`.split("\n")

export const mockEntries: TimelineEntry[] = [{
  date: weekAgo,
  description: "Created Workspace",
  title: "Admin",
}, {
  date: yesterday,
  description: "Modified Workspace",
  title: "Admin"
}, {
  date: today,
  description: "Modified Workspace",
  title: "Admin"
}, {
  date: today,
  description: "Restarted Workspace",
  title: "Admin"

}]

export interface TimelineEntryProps {
  entries: TimelineEntry[]
}

// Group timeline entry by date

const getDateWithoutTime = (date: Date) => {
  // TODO: Handle conversion to local time from UTC, as this may shift the actual day
  const dateWithoutTime = new Date(date.getTime())
  dateWithoutTime.setHours(0, 0, 0, 0)
  return dateWithoutTime
}

export const groupByDate = (entries: TimelineEntry[]): Record<string, TimelineEntry[]> => {
  const initial: Record<string, TimelineEntry[]> = {};
  return entries.reduce<Record<string, TimelineEntry[]>>((acc, curr) => {
    const dateWithoutTime = getDateWithoutTime(curr.date);
    const key = dateWithoutTime.getTime().toString()
    const currentEntry = acc[key];
    if (currentEntry) {
      return {
        ...acc,
        [key]: [...currentEntry, curr]
      }
    } else {
      return {
        ...acc,
        [key]: [curr]
      }
    }
  }, initial)

}

const formatDate = (date: Date) => {
  let formatter = new Intl.DateTimeFormat("en", {
    dateStyle: "long"
  });
  return formatter.format(date)
}

const formatTime = (date: Date) => {
  let formatter = new Intl.DateTimeFormat("en", {
    timeStyle: "short"
  });
  return formatter.format(date)
}



export interface EntryProps {
  entry: TimelineEntry
}

export const Entry: React.FC<EntryProps> = ({ entry }) => {
  const styles = useEntryStyles()
  const [expanded, setExpanded] = useState(false)

  const toggleExpanded = () => {
    setExpanded((prev: boolean) => !prev)
  }

  return <Box display={"flex"} flexDirection={"column"} onClick={toggleExpanded}>
    <Box display={"flex"} flexDirection={"row"} justifyContent={"flex-start"} alignItems={"center"}>
      <Box display={"flex"} flexDirection={"column"} justifyContent={"flex-start"} alignItems={"center"} mb={"auto"}>
        <Avatar>{"A"}</Avatar>
      </Box>
      <Box m={"0em 1em"} flexDirection={"column"} flex={"1"}>

        <Box display={"flex"} flexDirection={"row"} alignItems={"center"}>
          <Typography variant={"h6"}>{entry.title}</Typography>
          <Typography variant={"caption"} style={{ marginLeft: "1em" }}>{formatTime(entry.date)}</Typography>
        </Box>
        <Typography variant={"body2"}>{entry.description}</Typography>
        <Box>
          <BuildLog summary={"testing"} status={"success"} expanded={expanded} onToggleClicked={toggleExpanded} />
        </Box>
      </Box>
    </Box>

  </Box >
}

export const useEntryStyles = makeStyles((theme) => ({

}))

export type BuildLogStatus = "success" | "failure" | "pending"

export interface BuildLogProps {
  summary: string
  status: BuildLogStatus
  expanded?: boolean
}

export const BuildLog: React.FC<BuildLogProps> = ({ summary, status, expanded }) => {
  const styles = useBuildLogStyles(status)()

  return <div className={styles.container}>
    <button className={styles.collapseButton}>
      <Box m={"0.25em 0em"}>
        <Typography variant={"caption"}>{summary}</Typography>
        {expanded && <TerminalOutput output={sampleOutput} />}
      </Box>
    </button>
  </div>

}

const useBuildLogStyles = (status: BuildLogStatus) => makeStyles((theme) => ({
  container: {
    borderLeft: `2px solid ${status === "failure" ? theme.palette.error.main : theme.palette.info.main}`,
    margin: "1em 0em",
  },
  collapseButton: {
    color: "inherit",
    textAlign: "left",
    width: "100%",
    background: "none",
    border: 0,
    alignItems: "center",
    borderRadius: theme.spacing(0.5),
    cursor: "pointer",
    "&:disabled": {
      color: "inherit",
      cursor: "initial",
    },
    "&:hover:not(:disabled)": {
      backgroundColor: theme.palette.type === "dark" ? theme.palette.grey[800] : theme.palette.grey[100],
    },
  },
}))

export const Timeline: React.FC = () => {
  const styles = useStyles()

  const entries = mockEntries
  const groupedByDate = groupByDate(entries)
  const allDates = Object.keys(groupedByDate);
  const sortedDates = allDates.sort((a, b) => b.localeCompare(a))

  const days = sortedDates.map((date) => {

    const entriesForDay = groupedByDate[date];

    const entryElements = entriesForDay.map((entry) => <Entry entry={entry} isExpanded={false} />)


    return <div className={styles.root}>
      <Typography className={styles.header} variant="caption" color="textSecondary">{formatDate(new Date(Number.parseInt(date)))}</Typography>
      {entryElements}

    </div>
  })

  return <div className={styles.root}>
    {days}
  </div>

}

export const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    width: "100%",
    flexDirection: "column"
  },
  container: {
    display: "flex",
    flexDirection: "column",
  },
  header: {
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
    //textTransform: "uppercase"
  }
}))
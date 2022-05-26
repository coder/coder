import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import Link from "@material-ui/core/Link"
import Popover, { PopoverProps } from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Typography from "@material-ui/core/Typography"
import OpenInNewIcon from "@material-ui/icons/OpenInNew"
import Alert from "@material-ui/lab/Alert"
import React, { useState } from "react"
import { NetstatPort, NetstatResponse } from "../../api/types"
import { CodeExample } from "../CodeExample/CodeExample"
import { Stack } from "../Stack/Stack"

export const Language = {
  title: "Port forward",
  automaticPortText:
    "Here are the applications we detected are listening on ports in this resource. Click to open them in a new tab.",
  manualPortText:
    "You can manually port forward this resource by typing the port and your username in the URL like below.",
  formPortText: "Or you can use the following form to open the port in a new tab.",
  portListing: (port: number, name: string): string => `${port} (${name})`,
  portInputLabel: "Port",
  formButtonText: "Open URL",
}

export type PortForwardDropdownProps = Pick<PopoverProps, "onClose" | "open" | "anchorEl"> & {
  /**
   * The netstat response to render.  Undefined is taken to mean "loading".
   */
  netstat?: NetstatResponse
  /**
   * Given a port return the URL for accessing that port.
   */
  urlFormatter: (port: number | string) => string
}

const portFilter = ({ port }: NetstatPort): boolean => {
  if (port === 443 || port === 80) {
    // These are standard HTTP ports.
    return true
  } else if (port <= 1023) {
    // Assume a privileged port is probably not being used for HTTP.  This will
    // catch things like sshd.
    return false
  }
  return true
}

export const PortForwardDropdown: React.FC<PortForwardDropdownProps> = ({ netstat, open, urlFormatter, ...rest }) => {
  const styles = useStyles()
  const [port, setPort] = useState<number | string>(3000)
  const ports = netstat?.ports?.filter(portFilter)

  return (
    <Popover
      open={!!open}
      transformOrigin={{
        vertical: "top",
        horizontal: "center",
      }}
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "center",
      }}
      {...rest}
    >
      <div className={styles.root}>
        <Typography variant="h6" className={styles.title}>
          {Language.title}
        </Typography>

        <Typography className={styles.paragraph}>{Language.automaticPortText}</Typography>

        {typeof netstat === "undefined" && (
          <div className={styles.loader}>
            <CircularProgress size="1rem" />
          </div>
        )}

        {netstat?.error && <Alert severity="error">{netstat.error}</Alert>}

        {ports && ports.length > 0 && (
          <div className={styles.ports}>
            {ports.map(({ port, name }) => (
              <Link className={styles.portLink} key={port} href={urlFormatter(port)} target="_blank">
                <OpenInNewIcon />
                {Language.portListing(port, name)}
              </Link>
            ))}
          </div>
        )}

        {ports && ports.length === 0 && <Alert severity="info">No HTTP ports were detected.</Alert>}

        <Typography className={styles.paragraph}>{Language.manualPortText}</Typography>

        <CodeExample code={urlFormatter(port)} />

        <Typography className={styles.paragraph}>{Language.formPortText}</Typography>

        <Stack direction="row">
          <TextField
            className={styles.textField}
            onChange={(event) => setPort(event.target.value)}
            value={port}
            autoFocus
            label={Language.portInputLabel}
            variant="outlined"
          />
          <Button component={Link} href={urlFormatter(port)} target="_blank" className={styles.linkButton}>
            {Language.formButtonText}
          </Button>
        </Stack>
      </div>
    </Popover>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    padding: `${theme.spacing(3)}px`,
    maxWidth: 500,
  },
  title: {
    fontWeight: 600,
  },
  ports: {
    margin: `${theme.spacing(2)}px 0`,
  },
  portLink: {
    alignItems: "center",
    color: theme.palette.text.secondary,
    display: "flex",

    "& svg": {
      width: 16,
      height: 16,
      marginRight: theme.spacing(1.5),
    },
  },
  loader: {
    margin: `${theme.spacing(2)}px 0`,
    textAlign: "center",
  },
  paragraph: {
    color: theme.palette.text.secondary,
    margin: `${theme.spacing(2)}px 0`,
  },
  textField: {
    flex: 1,
    margin: 0,
  },
  linkButton: {
    color: "inherit",
    flex: 1,

    "&:hover": {
      textDecoration: "none",
    },
  },
}))

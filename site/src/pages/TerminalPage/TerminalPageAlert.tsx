import { AlertColor } from "@mui/material/Alert/Alert";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import { Alert } from "components/Alert/Alert";
import { ReactNode } from "react";
import { docs } from "utils/docs";

export type TerminalPageAlertType = "error" | "starting" | "success";

type MapAlertTypeToComponent = {
  [key in TerminalPageAlertType]: {
    severity: AlertColor;
    children: ReactNode | undefined;
  };
};

const mapAlertTypeToText: MapAlertTypeToComponent = {
  error: {
    severity: "warning",
    children: (
      <>
        The workspace{" "}
        <Link
          title="startup script has exited with an error"
          href={docs(
            "/templates/troubleshooting#startup-script-exited-with-an-error",
          )}
          target="_blank"
          rel="noreferrer"
        >
          startup script has exited with an error
        </Link>
        , we recommend reloading this session and{" "}
        <Link
          title=" debugging the startup script"
          href={docs("/templates/troubleshooting#debugging-the-startup-script")}
          target="_blank"
          rel="noreferrer"
        >
          debugging the startup script
        </Link>{" "}
        because{" "}
        <Link
          title="your workspace may be incomplete."
          href={docs(
            "/templates/troubleshooting#your-workspace-may-be-incomplete",
          )}
          target="_blank"
          rel="noreferrer"
        >
          your workspace may be incomplete.
        </Link>{" "}
      </>
    ),
  },
  starting: {
    severity: "info",
    children: (
      <>
        Startup script is still running. You can continue using this terminal,
        but{" "}
        <Link
          title="your workspace may be incomplete."
          href={docs(
            "/templates/troubleshooting#your-workspace-may-be-incomplete",
          )}
          target="_blank"
          rel="noreferrer"
        >
          {" "}
          your workspace may be incomplete.
        </Link>
      </>
    ),
  },
  success: {
    severity: "success",
    children: (
      <>
        Startup script has completed successfully. The workspace is ready but
        this{" "}
        <Link
          title="session was started before the startup script finished"
          href={docs(
            "/templates/troubleshooting#your-workspace-may-be-incomplete",
          )}
          target="_blank"
          rel="noreferrer"
        >
          session was started before the startup script finished.
        </Link>{" "}
        To ensure your shell environment is up-to-date, we recommend reloading
        this session.
      </>
    ),
  },
};

export default ({
  alertType,
  onDismiss,
}: {
  alertType: TerminalPageAlertType;
  onDismiss: () => void;
}) => {
  const severity = mapAlertTypeToText[alertType].severity;
  return (
    <Alert
      severity={severity}
      sx={{
        borderRadius: 0,
        borderWidth: 0,
        borderBottomWidth: 1,
        borderBottomColor: (theme) => theme.palette.divider,
        backgroundColor: (theme) => theme.palette.background.paper,
        borderLeft: (theme) => `3px solid ${theme.palette[severity].light}`,
        marginBottom: 1,
      }}
      onDismiss={onDismiss}
      dismissible
      actions={[
        <Button
          key="refresh-session"
          size="small"
          variant="text"
          onClick={() => {
            // By redirecting the user without the session in the URL we
            // create a new one
            window.location.href = window.location.pathname;
          }}
        >
          Refresh session
        </Button>,
      ]}
    >
      {mapAlertTypeToText[alertType].children}
    </Alert>
  );
};

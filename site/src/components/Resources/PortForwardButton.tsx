import Link from "@mui/material/Link";
import Popover from "@mui/material/Popover";
import { makeStyles } from "@mui/styles";
import { useRef, useState } from "react";
import { colors } from "theme/colors";
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { SecondaryAgentButton } from "components/Resources/AgentButton";
import { docs } from "utils/docs";
import Box from "@mui/material/Box";
import { useQuery } from "react-query";
import { getAgentListeningPorts } from "api/api";
import {
  WorkspaceAgent,
  WorkspaceAgentListeningPort,
} from "api/typesGenerated";
import CircularProgress from "@mui/material/CircularProgress";
import { portForwardURL } from "utils/portForward";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";

export interface PortForwardButtonProps {
  host: string;
  username: string;
  workspaceName: string;
  agent: WorkspaceAgent;
}

export const PortForwardButton: React.FC<PortForwardButtonProps> = (props) => {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "schedule-popover" : undefined;
  const styles = useStyles();
  const portsQuery = useQuery({
    queryKey: ["portForward", props.agent.id],
    queryFn: () => getAgentListeningPorts(props.agent.id),
    enabled: props.agent.status === "connected",
    refetchInterval: 5_000,
  });

  const onClose = () => {
    setIsOpen(false);
  };

  return (
    <>
      <SecondaryAgentButton
        disabled={!portsQuery.data}
        ref={anchorRef}
        onClick={() => {
          setIsOpen(true);
        }}
      >
        Ports
        {portsQuery.data ? (
          <Box
            sx={{
              fontSize: 12,
              fontWeight: 500,
              height: 20,
              minWidth: 20,
              padding: (theme) => theme.spacing(0, 0.5),
              borderRadius: "50%",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              backgroundColor: colors.gray[11],
              ml: 1,
            }}
          >
            {portsQuery.data.ports.length}
          </Box>
        ) : (
          <CircularProgress size={10} sx={{ ml: 1 }} />
        )}
      </SecondaryAgentButton>
      <Popover
        classes={{ paper: styles.popoverPaper }}
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={onClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
      >
        <PortForwardPopoverView {...props} ports={portsQuery.data?.ports} />
      </Popover>
    </>
  );
};

export const PortForwardPopoverView: React.FC<
  PortForwardButtonProps & { ports?: WorkspaceAgentListeningPort[] }
> = (props) => {
  const { host, workspaceName, agent, username, ports } = props;

  return (
    <>
      <Box
        sx={{
          padding: (theme) => theme.spacing(2.5),
          borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        }}
      >
        <HelpTooltipTitle>Forwarded ports</HelpTooltipTitle>
        <HelpTooltipText
          sx={{ color: (theme) => theme.palette.text.secondary }}
        >
          {ports?.length === 0
            ? "No open ports were detected."
            : "The forwarded ports are exclusively accessible to you."}
        </HelpTooltipText>
        <Box sx={{ marginTop: (theme) => theme.spacing(1.5) }}>
          {ports?.map((p) => {
            const url = portForwardURL(
              host,
              p.port,
              agent.name,
              workspaceName,
              username,
            );
            const label = p.process_name !== "" ? p.process_name : p.port;
            return (
              <Link
                underline="none"
                sx={{
                  color: (theme) => theme.palette.text.primary,
                  fontSize: 14,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  py: 0.5,
                  fontWeight: 500,
                }}
                key={p.port}
                href={url}
                target="_blank"
                rel="noreferrer"
              >
                <OpenInNewOutlined sx={{ width: 14, height: 14 }} />
                {label}
                <Box
                  component="span"
                  sx={{
                    ml: "auto",
                    color: (theme) => theme.palette.text.secondary,
                    fontSize: 13,
                    fontWeight: 400,
                  }}
                >
                  {p.port}
                </Box>
              </Link>
            );
          })}
        </Box>
      </Box>

      <Box
        sx={{
          padding: (theme) => theme.spacing(2.5),
        }}
      >
        <HelpTooltipTitle>Forward port</HelpTooltipTitle>
        <HelpTooltipText
          sx={{ color: (theme) => theme.palette.text.secondary }}
        >
          Access ports running on the agent:
        </HelpTooltipText>

        <Box
          component="form"
          sx={{
            border: (theme) => `1px solid ${theme.palette.divider}`,
            borderRadius: "4px",
            mt: 2,
            display: "flex",
            alignItems: "center",
            "&:focus-within": {
              borderColor: (theme) => theme.palette.primary.main,
            },
          }}
          onSubmit={(e) => {
            e.preventDefault();
            const formData = new FormData(e.currentTarget);
            const port = Number(formData.get("portNumber"));
            const url = portForwardURL(
              host,
              port,
              agent.name,
              workspaceName,
              username,
            );
            window.open(url, "_blank");
          }}
        >
          <Box
            aria-label="Port number"
            name="portNumber"
            component="input"
            type="number"
            placeholder="Type a port number..."
            min={0}
            max={65535}
            required
            sx={{
              fontSize: 14,
              height: 34,
              p: (theme) => theme.spacing(0, 1.5),
              background: "none",
              border: 0,
              outline: "none",
              color: (theme) => theme.palette.text.primary,
              appearance: "textfield",
              display: "block",
              width: "100%",
            }}
          />
          <OpenInNewOutlined
            sx={{
              flexShrink: 0,
              width: 14,
              height: 14,
              marginRight: (theme) => theme.spacing(1.5),
              color: (theme) => theme.palette.text.primary,
            }}
          />
        </Box>

        <HelpTooltipLinksGroup>
          <HelpTooltipLink href={docs("/networking/port-forwarding#dashboard")}>
            Learn more
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </Box>
    </>
  );
};

const useStyles = makeStyles((theme) => ({
  popoverPaper: {
    padding: 0,
    width: theme.spacing(38),
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },

  openUrlButton: {
    flexShrink: 0,
  },

  portField: {
    // The default border don't contrast well with the popover
    "& .MuiOutlinedInput-root .MuiOutlinedInput-notchedOutline": {
      borderColor: colors.gray[10],
    },
  },

  code: {
    margin: theme.spacing(2, 0),
  },

  form: {
    margin: theme.spacing(2, 0),
  },
}));

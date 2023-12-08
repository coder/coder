import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import CircularProgress from "@mui/material/CircularProgress";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { docs } from "utils/docs";
import { getAgentListeningPorts } from "api/api";
import type {
  WorkspaceAgent,
  WorkspaceAgentListeningPort,
} from "api/typesGenerated";
import { portForwardURL } from "utils/portForward";
import { type ClassName, useClassName } from "hooks/useClassName";
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { AgentButton } from "components/Resources/AgentButton";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { DisplayAppNameMap } from "./AppLink/AppLink";

export interface PortForwardButtonProps {
  host: string;
  username: string;
  workspaceName: string;
  agent: WorkspaceAgent;
}

export const PortForwardButton: FC<PortForwardButtonProps> = (props) => {
  const { agent } = props;

  const paper = useClassName(classNames.paper, []);

  const portsQuery = useQuery({
    queryKey: ["portForward", agent.id],
    queryFn: () => getAgentListeningPorts(agent.id),
    enabled: agent.status === "connected",
    refetchInterval: 5_000,
  });

  return (
    <Popover>
      <PopoverTrigger>
        <AgentButton disabled={!portsQuery.data}>
          {DisplayAppNameMap["port_forwarding_helper"]}
          {portsQuery.data ? (
            <div css={styles.portCount}>{portsQuery.data.ports.length}</div>
          ) : (
            <CircularProgress size={10} css={{ marginLeft: 8 }} />
          )}
        </AgentButton>
      </PopoverTrigger>
      <PopoverContent horizontal="right" classes={{ paper }}>
        <PortForwardPopoverView {...props} ports={portsQuery.data?.ports} />
      </PopoverContent>
    </Popover>
  );
};

interface PortForwardPopoverViewProps extends PortForwardButtonProps {
  ports?: WorkspaceAgentListeningPort[];
}

export const PortForwardPopoverView: FC<PortForwardPopoverViewProps> = ({
  host,
  workspaceName,
  agent,
  username,
  ports,
}) => {
  const theme = useTheme();

  return (
    <>
      <div
        css={{
          padding: 20,
          borderBottom: `1px solid ${theme.palette.divider}`,
        }}
      >
        <HelpTooltipTitle>Forwarded ports</HelpTooltipTitle>
        <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
          {ports?.length === 0
            ? "No open ports were detected."
            : "The forwarded ports are exclusively accessible to you."}
        </HelpTooltipText>
        <div css={{ marginTop: 12 }}>
          {ports?.map((port) => {
            const url = portForwardURL(
              host,
              port.port,
              agent.name,
              workspaceName,
              username,
            );
            const label =
              port.process_name !== "" ? port.process_name : port.port;
            return (
              <Link
                underline="none"
                css={styles.portLink}
                key={port.port}
                href={url}
                target="_blank"
                rel="noreferrer"
              >
                <OpenInNewOutlined css={{ width: 14, height: 14 }} />
                {label}
                <span css={styles.portNumber}>{port.port}</span>
              </Link>
            );
          })}
        </div>
      </div>

      <div css={{ padding: 20 }}>
        <HelpTooltipTitle>Forward port</HelpTooltipTitle>
        <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
          Access ports running on the agent:
        </HelpTooltipText>

        <form
          css={styles.newPortForm}
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
          <input
            aria-label="Port number"
            name="portNumber"
            type="number"
            placeholder="Type a port number..."
            min={0}
            max={65535}
            required
            css={styles.newPortInput}
          />
          <Button
            type="submit"
            size="small"
            variant="text"
            css={{
              paddingLeft: 12,
              paddingRight: 12,
              minWidth: 0,
            }}
          >
            <OpenInNewOutlined
              css={{
                flexShrink: 0,
                width: 14,
                height: 14,
                color: theme.palette.text.primary,
              }}
            />
          </Button>
        </form>

        <HelpTooltipLinksGroup>
          <HelpTooltipLink href={docs("/networking/port-forwarding#dashboard")}>
            Learn more
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </div>
    </>
  );
};

const classNames = {
  paper: (css, theme) => css`
    padding: 0;
    width: 304px;
    color: ${theme.palette.text.secondary};
    margin-top: 4px;
  `,
} satisfies Record<string, ClassName>;

const styles = {
  portCount: (theme) => ({
    fontSize: 12,
    fontWeight: 500,
    height: 20,
    minWidth: 20,
    padding: "0 4px",
    borderRadius: "50%",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: theme.colors.gray[11],
    marginLeft: 8,
  }),

  portLink: (theme) => ({
    color: theme.palette.text.primary,
    fontSize: 14,
    display: "flex",
    alignItems: "center",
    gap: 8,
    paddingTop: 4,
    paddingBottom: 4,
    fontWeight: 500,
  }),

  portNumber: (theme) => ({
    marginLeft: "auto",
    color: theme.palette.text.secondary,
    fontSize: 13,
    fontWeight: 400,
  }),

  newPortForm: (theme) => ({
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: "4px",
    marginTop: 16,
    display: "flex",
    alignItems: "center",
    "&:focus-within": {
      borderColor: theme.palette.primary.main,
    },
  }),

  newPortInput: (theme) => ({
    fontSize: 14,
    height: 34,
    padding: "0 12px",
    background: "none",
    border: 0,
    outline: "none",
    color: theme.palette.text.primary,
    appearance: "textfield",
    display: "block",
    width: "100%",
  }),
} satisfies Record<string, Interpolation<Theme>>;

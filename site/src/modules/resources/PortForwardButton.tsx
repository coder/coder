import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import CircularProgress from "@mui/material/CircularProgress";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { docs } from "utils/docs";
import { getAgentListeningPorts, getWorkspaceAgentSharedPorts } from "api/api";
import type {
  WorkspaceAgent,
  WorkspaceAgentListeningPort,
  WorkspaceAgentListeningPortsResponse,
  WorkspaceAgentPortShare,
  WorkspaceAgentPortShares,
} from "api/typesGenerated";
import { portForwardURL } from "utils/portForward";
import { type ClassName, useClassName } from "hooks/useClassName";
import {
  HelpTooltipLink,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import Stack from "@mui/material/Stack";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import FormControl from "@mui/material/FormControl";
import TextField from "@mui/material/TextField";
import SensorsIcon from "@mui/icons-material/Sensors";
import LockIcon from "@mui/icons-material/Lock";
import LockOpenIcon from "@mui/icons-material/LockOpen";
import IconButton from "@mui/material/IconButton";
import CloseIcon from "@mui/icons-material/Close";
import Grid from "@mui/material/Grid";

export interface PortForwardButtonProps {
  host: string;
  username: string;
  workspaceName: string;
  workspaceID: string;
  agent: WorkspaceAgent;

  /**
   * Only for use in Storybook
   */
  storybook?: {
    listeningPortsQueryData?: WorkspaceAgentListeningPortsResponse;
    sharedPortsQueryData?: WorkspaceAgentPortShares;
  };
}

export const PortForwardButton: FC<PortForwardButtonProps> = (props) => {
  const { agent, workspaceID, storybook } = props;

  const paper = useClassName(classNames.paper, []);

  const portsQuery = useQuery({
    queryKey: ["portForward", agent.id],
    queryFn: () => getAgentListeningPorts(agent.id),
    enabled: !storybook && agent.status === "connected",
    refetchInterval: 5_000,
  });

  const sharedPortsQuery = useQuery({
    queryKey: ["sharedPorts", agent.id],
    queryFn: () => getWorkspaceAgentSharedPorts(workspaceID),
    enabled: !storybook && agent.status === "connected",
  });

  const listeningPorts = storybook
    ? storybook.listeningPortsQueryData
    : portsQuery.data;
  const sharedPorts = storybook
    ? storybook.sharedPortsQueryData
    : sharedPortsQuery.data;

  return (
    <Popover>
      <PopoverTrigger>
        <Button
          disabled={!listeningPorts}
          size="small"
          variant="text"
          endIcon={<KeyboardArrowDown />}
          css={{ fontSize: 13, padding: "8px 12px" }}
          startIcon={
            listeningPorts ? (
              <div>
                <span css={styles.portCount}>
                  {listeningPorts.ports.length}
                </span>
              </div>
            ) : (
              <CircularProgress size={10} />
            )
          }
        >
          Open ports
        </Button>
      </PopoverTrigger>
      <PopoverContent horizontal="right" classes={{ paper }}>
        <PortForwardPopoverView
          {...props}
          listeningPorts={listeningPorts?.ports}
          sharedPorts={sharedPorts?.shares}
        />
      </PopoverContent>
    </Popover>
  );
};

interface PortForwardPopoverViewProps extends PortForwardButtonProps {
  listeningPorts?: WorkspaceAgentListeningPort[];
  sharedPorts?: WorkspaceAgentPortShare[];
}

export const PortForwardPopoverView: FC<PortForwardPopoverViewProps> = ({
  host,
  workspaceName,
  agent,
  username,
  listeningPorts,
  sharedPorts,
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
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="start"
        >
          <HelpTooltipTitle>Listening ports</HelpTooltipTitle>
          <HelpTooltipLink href={docs("/networking/port-forwarding#dashboard")}>
            Learn more
          </HelpTooltipLink>
        </Stack>
        <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
          {listeningPorts?.length === 0
            ? "No open ports were detected."
            : "The listening ports are exclusively accessible to you."}
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
            placeholder="Connect to port..."
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
        <div
          css={{
            paddingTop: 10,
          }}
        >
          {listeningPorts?.map((port) => {
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
              <Stack
                key={port.port}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
              >
                <Link
                  underline="none"
                  css={styles.portLink}
                  href={url}
                  target="_blank"
                  rel="noreferrer"
                >
                  <SensorsIcon css={{ width: 14, height: 14 }} />
                  {label}
                </Link>
                <Stack direction="row" gap={2} justifyContent="flex-end" alignItems="center">
                <Link
                  underline="none"
                  css={styles.portLink}
                  href={url}
                  target="_blank"
                  rel="noreferrer"
                >
                  <span css={styles.portNumber}>{port.port}</span>
                </Link>
                <Button size="small" variant="text">
                  Share
                </Button>
                </Stack>
              </Stack>
            );
          })}
        </div>
      </div>
      <div
        css={{
          padding: 20,
        }}
      >
        <HelpTooltipTitle>Shared Ports</HelpTooltipTitle>
        <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
          Ports can be shared with other Coder users or with the public.
        </HelpTooltipText>
        <div>
          {sharedPorts?.map((share) => {
            const url = portForwardURL(
              host,
              share.port,
              agent.name,
              workspaceName,
              username,
            );
            const label = share.port;
            return (
              <Stack
                key={share.port}
                direction="row"
                justifyContent="space-between"
                alignItems="center"
              >
                <Link
                  underline="none"
                  css={styles.portLink}
                  href={url}
                  target="_blank"
                  rel="noreferrer"
                >
                  {share.share_level === "public" ? (
                    <LockOpenIcon css={{ width: 14, height: 14 }} />
                  ) : (
                    <LockIcon css={{ width: 14, height: 14 }} />
                  )}
                  {label}
                </Link>
                <Stack direction="row" justifyContent="flex-end">
                <FormControl size="small">
                  <Select
                    sx={{
                      boxShadow: "none",
                      ".MuiOutlinedInput-notchedOutline": { border: 0 },
                      "&.MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline":
                        {
                          border: 0,
                        },
                      "&.MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline":
                        {
                          border: 0,
                        },
                    }}
                    value={share.share_level}
                  >
                    <MenuItem value="authenticated">Authenticated</MenuItem>
                    <MenuItem value="public">Public</MenuItem>
                  </Select>
                </FormControl>
                <IconButton>
                  <CloseIcon
                    css={{
                      width: 14,
                      height: 14,
                      color: theme.palette.text.primary,
                    }}
                  />
                </IconButton>
                </Stack>
              </Stack>
            );
          })}
        </div>
        <Stack
          direction="column"
          gap={1}
          justifyContent="flex-end"
          sx={{
            marginTop: 2,
          }}
        >
          <TextField label="Port" variant="outlined" size="small" />
          <FormControl size="small">
            <Select value="Authenticated">
              <MenuItem value="Authenticated">Authenticated</MenuItem>
              <MenuItem value="Public">Public</MenuItem>
            </Select>
          </FormControl>
          <Button variant="contained">Share Port</Button>
        </Stack>
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
    backgroundColor: theme.palette.action.selected,
  }),

  portLink: (theme) => ({
    color: theme.palette.text.primary,
    fontSize: 14,
    display: "flex",
    alignItems: "center",
    gap: 8,
    paddingTop: 8,
    paddingBottom: 8,
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
    marginTop: 8,
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

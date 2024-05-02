import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import CloseIcon from "@mui/icons-material/Close";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import LockIcon from "@mui/icons-material/Lock";
import LockOpenIcon from "@mui/icons-material/LockOpen";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import SensorsIcon from "@mui/icons-material/Sensors";
import LoadingButton from "@mui/lab/LoadingButton";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import FormControl from "@mui/material/FormControl";
import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import { type FormikContextType, useFormik } from "formik";
import { useState, type FC } from "react";
import { useQuery, useMutation } from "react-query";
import * as Yup from "yup";
import { getAgentListeningPorts } from "api/api";
import {
  deleteWorkspacePortShare,
  upsertWorkspacePortShare,
  workspacePortShares,
} from "api/queries/workspaceportsharing";
import {
  type Template,
  type WorkspaceAgent,
  type WorkspaceAgentListeningPort,
  type WorkspaceAgentPortShareLevel,
  type UpsertWorkspaceAgentPortShareRequest,
  type WorkspaceAgentPortShareProtocol,
  WorkspaceAppSharingLevels,
} from "api/typesGenerated";
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
import { type ClassName, useClassName } from "hooks/useClassName";
import { useDashboard } from "modules/dashboard/useDashboard";
import { docs } from "utils/docs";
import { getFormHelpers } from "utils/formUtils";
import {
  getWorkspaceListeningPortsProtocol,
  portForwardURL,
  saveWorkspaceListeningPortsProtocol,
} from "utils/portForward";

export interface PortForwardButtonProps {
  host: string;
  username: string;
  workspaceName: string;
  workspaceID: string;
  agent: WorkspaceAgent;
  template: Template;
}

export const PortForwardButton: FC<PortForwardButtonProps> = (props) => {
  const { agent } = props;
  const { entitlements } = useDashboard();
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
        <Button
          disabled={!portsQuery.data}
          size="small"
          variant="text"
          endIcon={<KeyboardArrowDown />}
          css={{ fontSize: 13, padding: "8px 12px" }}
          startIcon={
            portsQuery.data ? (
              <div>
                <span css={styles.portCount}>
                  {portsQuery.data.ports.length}
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
          listeningPorts={portsQuery.data?.ports}
          portSharingControlsEnabled={
            entitlements.features.control_shared_ports.enabled
          }
        />
      </PopoverContent>
    </Popover>
  );
};

const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    port: Yup.number().required().min(9).max(65535),
    share_level: Yup.string().required().oneOf(WorkspaceAppSharingLevels),
  });

interface PortForwardPopoverViewProps extends PortForwardButtonProps {
  listeningPorts?: readonly WorkspaceAgentListeningPort[];
  portSharingControlsEnabled: boolean;
}

type Optional<T, K extends keyof T> = Pick<Partial<T>, K> & Omit<T, K>;

export const PortForwardPopoverView: FC<PortForwardPopoverViewProps> = ({
  host,
  workspaceName,
  workspaceID,
  agent,
  template,
  username,
  listeningPorts,
  portSharingControlsEnabled,
}) => {
  const theme = useTheme();
  const [listeningPortProtocol, setListeningPortProtocol] = useState(
    getWorkspaceListeningPortsProtocol(workspaceID),
  );

  const sharedPortsQuery = useQuery({
    ...workspacePortShares(workspaceID),
    enabled: agent.status === "connected",
  });
  const sharedPorts = sharedPortsQuery.data?.shares || [];

  const upsertSharedPortMutation = useMutation(
    upsertWorkspacePortShare(workspaceID),
  );

  const deleteSharedPortMutation = useMutation(
    deleteWorkspacePortShare(workspaceID),
  );

  // share port form
  const {
    mutateAsync: upsertWorkspacePortShareForm,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(upsertWorkspacePortShare(workspaceID));
  const validationSchema = getValidationSchema();
  // TODO: do partial here
  const form: FormikContextType<
    Optional<UpsertWorkspaceAgentPortShareRequest, "port">
  > = useFormik<Optional<UpsertWorkspaceAgentPortShareRequest, "port">>({
    initialValues: {
      agent_name: agent.name,
      port: undefined,
      protocol: "http",
      share_level: "authenticated",
    },
    validationSchema,
    onSubmit: async (values) => {
      // we need port to be optional in the initialValues so it appears empty instead of 0.
      // because of this we need to reset the form to clear the port field manually.
      form.resetForm();
      await form.setFieldValue("port", "");

      const port = Number(values.port);
      await upsertWorkspacePortShareForm({
        ...values,
        port,
      });
      await sharedPortsQuery.refetch();
    },
  });
  const getFieldHelpers = getFormHelpers(form, submitError);

  // filter out shared ports that are not from this agent
  const filteredSharedPorts = sharedPorts.filter(
    (port) => port.agent_name === agent.name,
  );
  // we don't want to show listening ports if it's a shared port
  const filteredListeningPorts = (listeningPorts ?? []).filter((port) =>
    filteredSharedPorts.every((sharedPort) => sharedPort.port !== port.port),
  );
  // only disable the form if shared port controls are entitled and the template doesn't allow sharing ports
  const canSharePorts = !(
    portSharingControlsEnabled && template.max_port_share_level === "owner"
  );
  const canSharePortsPublic =
    canSharePorts && template.max_port_share_level === "public";

  const disabledPublicMenuItem = (
    <Tooltip title="This workspace template does not allow sharing ports with unauthenticated users.">
      {/* Tooltips don't work directly on disabled MenuItem components so you must wrap in div. */}
      <div>
        <MenuItem value="public" disabled>
          Public
        </MenuItem>
      </div>
    </Tooltip>
  );

  return (
    <>
      <div
        css={{
          maxHeight: 320,
          overflowY: "auto",
        }}
      >
        <Stack
          direction="column"
          css={{
            padding: 20,
          }}
        >
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="start"
          >
            <HelpTooltipTitle>Listening Ports</HelpTooltipTitle>
            <HelpTooltipLink
              href={docs("/networking/port-forwarding#dashboard")}
            >
              Learn more
            </HelpTooltipLink>
          </Stack>
          <Stack direction="column" gap={1}>
            <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
              The listening ports are exclusively accessible to you. Selecting
              HTTP/S will change the protocol for all listening ports.
            </HelpTooltipText>
            <Stack
              direction="row"
              gap={2}
              css={{
                paddingBottom: 8,
              }}
            >
              <FormControl size="small" css={styles.protocolFormControl}>
                <Select
                  css={styles.listeningPortProtocol}
                  value={listeningPortProtocol}
                  onChange={async (event) => {
                    const selectedProtocol = event.target.value as
                      | "http"
                      | "https";
                    setListeningPortProtocol(selectedProtocol);
                    saveWorkspaceListeningPortsProtocol(
                      workspaceID,
                      selectedProtocol,
                    );
                  }}
                >
                  <MenuItem value="http">HTTP</MenuItem>
                  <MenuItem value="https">HTTPS</MenuItem>
                </Select>
              </FormControl>
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
                    listeningPortProtocol,
                  );
                  window.open(url, "_blank");
                }}
              >
                <input
                  aria-label="Port number"
                  name="portNumber"
                  type="number"
                  placeholder="Connect to port..."
                  min={9}
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
            </Stack>
          </Stack>
          {filteredListeningPorts.length === 0 && (
            <HelpTooltipText css={styles.noPortText}>
              No open ports were detected.
            </HelpTooltipText>
          )}
          {filteredListeningPorts.map((port) => {
            const url = portForwardURL(
              host,
              port.port,
              agent.name,
              workspaceName,
              username,
              listeningPortProtocol,
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
                <Stack direction="row" gap={3}>
                  <Link
                    underline="none"
                    css={styles.portLink}
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <SensorsIcon css={{ width: 14, height: 14 }} />
                    {port.port}
                  </Link>
                  <Link
                    underline="none"
                    css={styles.portLink}
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {label}
                  </Link>
                </Stack>
                <Stack
                  direction="row"
                  gap={2}
                  justifyContent="flex-end"
                  alignItems="center"
                >
                  {canSharePorts && (
                    <Button
                      size="small"
                      variant="text"
                      onClick={async () => {
                        await upsertSharedPortMutation.mutateAsync({
                          agent_name: agent.name,
                          port: port.port,
                          protocol: listeningPortProtocol,
                          share_level: "authenticated",
                        });
                        await sharedPortsQuery.refetch();
                      }}
                    >
                      Share
                    </Button>
                  )}
                </Stack>
              </Stack>
            );
          })}
        </Stack>
      </div>
      <div
        css={{
          padding: 20,
          borderTop: `1px solid ${theme.palette.divider}`,
        }}
      >
        <HelpTooltipTitle>Shared Ports</HelpTooltipTitle>
        <HelpTooltipText css={{ color: theme.palette.text.secondary }}>
          {canSharePorts
            ? "Ports can be shared with other Coder users or with the public."
            : "This workspace template does not allow sharing ports. Contact a template administrator to enable port sharing."}
        </HelpTooltipText>
        {canSharePorts && (
          <div>
            {filteredSharedPorts?.map((share) => {
              const url = portForwardURL(
                host,
                share.port,
                agent.name,
                workspaceName,
                username,
                share.protocol,
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
                  <FormControl size="small" css={styles.protocolFormControl}>
                    <Select
                      css={styles.shareLevelSelect}
                      value={share.protocol}
                      onChange={async (event) => {
                        await upsertSharedPortMutation.mutateAsync({
                          agent_name: agent.name,
                          port: share.port,
                          protocol: event.target
                            .value as WorkspaceAgentPortShareProtocol,
                          share_level: share.share_level,
                        });
                        await sharedPortsQuery.refetch();
                      }}
                    >
                      <MenuItem value="http">HTTP</MenuItem>
                      <MenuItem value="https">HTTPS</MenuItem>
                    </Select>
                  </FormControl>

                  <Stack direction="row" justifyContent="flex-end">
                    <FormControl
                      size="small"
                      css={styles.shareLevelFormControl}
                    >
                      <Select
                        css={styles.shareLevelSelect}
                        value={share.share_level}
                        onChange={async (event) => {
                          await upsertSharedPortMutation.mutateAsync({
                            agent_name: agent.name,
                            port: share.port,
                            protocol: share.protocol,
                            share_level: event.target
                              .value as WorkspaceAgentPortShareLevel,
                          });
                          await sharedPortsQuery.refetch();
                        }}
                      >
                        <MenuItem value="authenticated">Authenticated</MenuItem>
                        {canSharePortsPublic ? (
                          <MenuItem value="public">Public</MenuItem>
                        ) : (
                          disabledPublicMenuItem
                        )}
                      </Select>
                    </FormControl>
                    <Button
                      size="small"
                      variant="text"
                      css={styles.deleteButton}
                      onClick={async () => {
                        await deleteSharedPortMutation.mutateAsync({
                          agent_name: agent.name,
                          port: share.port,
                        });
                        await sharedPortsQuery.refetch();
                      }}
                    >
                      <CloseIcon
                        css={{
                          width: 14,
                          height: 14,
                          color: theme.palette.text.primary,
                        }}
                      />
                    </Button>
                  </Stack>
                </Stack>
              );
            })}
            <form onSubmit={form.handleSubmit}>
              <Stack
                direction="column"
                gap={2}
                justifyContent="flex-end"
                sx={{
                  marginTop: 2,
                }}
              >
                <TextField
                  {...getFieldHelpers("port")}
                  disabled={isSubmitting}
                  label="Port"
                  size="small"
                  variant="outlined"
                  type="number"
                  value={form.values.port}
                />
                <TextField
                  {...getFieldHelpers("protocol")}
                  disabled={isSubmitting}
                  fullWidth
                  select
                  value={form.values.protocol}
                  label="Protocol"
                >
                  <MenuItem value="http">HTTP</MenuItem>
                  <MenuItem value="https">HTTPS</MenuItem>
                </TextField>
                <TextField
                  {...getFieldHelpers("share_level")}
                  disabled={isSubmitting}
                  fullWidth
                  select
                  value={form.values.share_level}
                  label="Sharing Level"
                >
                  <MenuItem value="authenticated">Authenticated</MenuItem>
                  {canSharePortsPublic ? (
                    <MenuItem value="public">Public</MenuItem>
                  ) : (
                    disabledPublicMenuItem
                  )}
                </TextField>
                <LoadingButton
                  variant="contained"
                  type="submit"
                  loading={isSubmitting}
                  disabled={!form.isValid}
                >
                  Share Port
                </LoadingButton>
              </Stack>
            </form>
          </div>
        )}
      </div>
    </>
  );
};

const classNames = {
  paper: (css, theme) => css`
    padding: 0;
    width: 404px;
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
    minWidth: 80,
  }),

  portNumber: (theme) => ({
    marginLeft: "auto",
    color: theme.palette.text.secondary,
    fontSize: 13,
    fontWeight: 400,
  }),

  shareLevelSelect: () => ({
    boxShadow: "none",
    ".MuiOutlinedInput-notchedOutline": { border: 0 },
    "&.MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline": {
      border: 0,
    },
    "&.MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline": {
      border: 0,
    },
  }),

  deleteButton: () => ({
    minWidth: 30,
    padding: 0,
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
    width: "100%",
  }),

  listeningPortProtocol: (theme) => ({
    boxShadow: "none",
    ".MuiOutlinedInput-notchedOutline": { border: 0 },
    "&.MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline": {
      border: 0,
    },
    "&.MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline": {
      border: 0,
    },
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: "4px",
    marginTop: 8,
    minWidth: "100px",
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
  noPortText: (theme) => ({
    color: theme.palette.text.secondary,
    paddingTop: 20,
    paddingBottom: 10,
    textAlign: "center",
  }),
  sharedPortLink: () => ({
    minWidth: 80,
  }),
  protocolFormControl: () => ({
    minWidth: 90,
  }),
  shareLevelFormControl: () => ({
    minWidth: 140,
  }),
} satisfies Record<string, Interpolation<Theme>>;

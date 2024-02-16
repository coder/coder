import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import CircularProgress from "@mui/material/CircularProgress";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC } from "react";
import { useQuery, useMutation } from "react-query";
import { docs } from "utils/docs";
import { getAgentListeningPorts } from "api/api";
import {
  WorkspaceAppSharingLevels,
  type Template,
  type WorkspaceAgent,
  type WorkspaceAgentListeningPort,
  type WorkspaceAgentListeningPortsResponse,
  type WorkspaceAgentPortShareLevel,
  type WorkspaceAgentPortShares,
  UpsertWorkspaceAgentPortShareRequest,
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
import CloseIcon from "@mui/icons-material/Close";
import {
  deleteWorkspacePortShare,
  upsertWorkspacePortShare,
  workspacePortShares,
} from "api/queries/workspaceportsharing";
import { useDashboard } from "modules/dashboard/useDashboard";
import * as Yup from "yup";
import { FormikContextType, useFormik } from "formik";
import { getFormHelpers } from "utils/formUtils";

export interface PortForwardButtonProps {
  host: string;
  username: string;
  workspaceName: string;
  workspaceID: string;
  agent: WorkspaceAgent;
  template: Template;

  /**
   * Only for use in Storybook
   */
  storybook?: {
    listeningPortsQueryData?: WorkspaceAgentListeningPortsResponse;
    sharedPortsQueryData?: WorkspaceAgentPortShares;
  };
}

export const PortForwardButton: FC<PortForwardButtonProps> = (props) => {
  const { agent, storybook } = props;
  const { entitlements, experiments } = useDashboard();
  const paper = useClassName(classNames.paper, []);

  const portsQuery = useQuery({
    queryKey: ["portForward", agent.id],
    queryFn: () => getAgentListeningPorts(agent.id),
    enabled: !storybook && agent.status === "connected",
    refetchInterval: 5_000,
  });

  const listeningPorts = storybook
    ? storybook.listeningPortsQueryData
    : portsQuery.data;

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
          portSharingExperimentEnabled={experiments.includes("shared-ports")}
          portSharingControlsEnabled={
            entitlements.features.control_shared_ports.enabled
          }
        />
      </PopoverContent>
    </Popover>
  );
};

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    port: Yup.number().required().min(0).max(65535),
    sharing_level: Yup.string().required().oneOf(WorkspaceAppSharingLevels),
  });

interface PortForwardPopoverViewProps extends PortForwardButtonProps {
  listeningPorts?: WorkspaceAgentListeningPort[];
  portSharingExperimentEnabled: boolean;
  portSharingControlsEnabled: boolean;
}

export const PortForwardPopoverView: FC<PortForwardPopoverViewProps> = ({
  host,
  workspaceName,
  workspaceID,
  agent,
  template,
  username,
  listeningPorts,
  portSharingExperimentEnabled,
  portSharingControlsEnabled,
  storybook,
}) => {
  const theme = useTheme();

  const sharedPortsQuery = useQuery({
    ...workspacePortShares(workspaceID),
    enabled: !storybook && agent.status === "connected",
  });
  const sharedPorts = storybook
    ? storybook.sharedPortsQueryData?.shares || []
    : sharedPortsQuery.data?.shares || [];

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
  const form: FormikContextType<UpsertWorkspaceAgentPortShareRequest> =
    useFormik<UpsertWorkspaceAgentPortShareRequest>({
      initialValues: {
        agent_name: agent.name,
        port: 0,
        share_level: "authenticated",
      },
      validationSchema,
      onSubmit: async (values) => {
        await upsertWorkspacePortShareForm(values);
        await sharedPortsQuery.refetch();
      },
    });
  const getFieldHelpers = getFormHelpers(form, submitError);

  // we don't want to show listening ports if it's a shared port
  const filteredListeningPorts = listeningPorts?.filter((port) => {
    for (let i = 0; i < sharedPorts.length; i++) {
      if (
        sharedPorts[i].port === port.port &&
        sharedPorts[i].agent_name === agent.name
      ) {
        return false;
      }
    }

    return true;
  });
  // only disable the form if shared port controls are entitled and the template doesn't allow sharing ports
  const canSharePorts =
    portSharingExperimentEnabled &&
    !(portSharingControlsEnabled && template.max_port_share_level === "owner");
  const canSharePortsPublic =
    canSharePorts && template.max_port_share_level === "public";

  return (
    <>
      <div
        css={{
          padding: 20,
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
          {filteredListeningPorts?.length === 0
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
          {filteredListeningPorts?.map((port) => {
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
                <Stack
                  direction="row"
                  gap={2}
                  justifyContent="flex-end"
                  alignItems="center"
                >
                  <Link
                    underline="none"
                    css={styles.portLink}
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <span css={styles.portNumber}>{port.port}</span>
                  </Link>
                  {canSharePorts && (
                    <Button
                      size="small"
                      variant="text"
                      onClick={async () => {
                        await upsertSharedPortMutation.mutateAsync({
                          agent_name: agent.name,
                          port: port.port,
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
        </div>
      </div>
      {portSharingExperimentEnabled && (
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
                          css={styles.shareLevelSelect}
                          value={share.share_level}
                          onChange={async (event) => {
                            await upsertSharedPortMutation.mutateAsync({
                              agent_name: agent.name,
                              port: share.port,
                              share_level: event.target
                                .value as WorkspaceAgentPortShareLevel,
                            });
                            await sharedPortsQuery.refetch();
                          }}
                        >
                          <MenuItem value="authenticated">
                            Authenticated
                          </MenuItem>
                          <MenuItem
                            value="public"
                            disabled={!canSharePortsPublic}
                          >
                            Public
                          </MenuItem>
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
              <form onSubmit={form.submitForm}>
                <Stack
                  direction="column"
                  gap={1}
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
                  />
                  <FormControl size="small">
                    <Select
                      {...getFieldHelpers("share_level")}
                      value={form.values.share_level}
                      onChange={form.handleChange}
                    >
                      <MenuItem value="authenticated">Authenticated</MenuItem>
                      <MenuItem value="public" disabled={!canSharePortsPublic}>
                        Public
                      </MenuItem>
                    </Select>
                  </FormControl>
                  <Button variant="contained" onClick={form.submitForm}>
                    Share Port
                  </Button>
                </Stack>
              </form>
            </div>
          )}
        </div>
      )}
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

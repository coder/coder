import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import FormControl from "@mui/material/FormControl";
import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import { API } from "api/api";
import {
	deleteWorkspacePortShare,
	upsertWorkspacePortShare,
	workspacePortShares,
} from "api/queries/workspaceportsharing";
import {
	type Template,
	type Workspace,
	type WorkspaceAgent,
	type WorkspaceAgentListeningPort,
	type WorkspaceAgentPortShare,
	type WorkspaceAgentPortShareLevel,
	type WorkspaceAgentPortShareProtocol,
	WorkspaceAppSharingLevels,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
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
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useFormik } from "formik";
import {
	BuildingIcon,
	ChevronDownIcon,
	ExternalLinkIcon,
	LockIcon,
	LockOpenIcon,
	RadioIcon,
	ShareIcon,
	X as XIcon,
} from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { docs } from "utils/docs";
import { getFormHelpers } from "utils/formUtils";
import {
	getWorkspaceListeningPortsProtocol,
	portForwardURL,
	saveWorkspaceListeningPortsProtocol,
} from "utils/portForward";
import * as Yup from "yup";

interface PortForwardButtonProps {
	host: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	template: Template;
}

export const PortForwardButton: FC<PortForwardButtonProps> = ({
	host,
	workspace,
	template,
	agent,
}) => {
	const { entitlements } = useDashboard();

	const { data: listeningPorts } = useQuery({
		queryKey: ["portForward", agent.id],
		queryFn: () => API.getAgentListeningPorts(agent.id),
		enabled: agent.status === "connected",
		refetchInterval: 5_000,
		select: (res) => res.ports,
	});

	const { data: sharedPorts, refetch: refetchSharedPorts } = useQuery({
		...workspacePortShares(workspace.id),
		enabled: agent.status === "connected",
		select: (res) => res.shares,
	});

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button disabled={!listeningPorts} size="sm" variant="subtle">
					<Spinner loading={!listeningPorts}>
						<span css={styles.portCount}>{listeningPorts?.length}</span>
					</Spinner>
					Open ports
					<ChevronDownIcon className="size-4" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="p-0 w-[404px] mt-1 text-content-secondary bg-surface-secondary border-surface-quaternary"
			>
				<PortForwardPopoverView
					host={host}
					agent={agent}
					workspace={workspace}
					template={template}
					sharedPorts={sharedPorts ?? []}
					listeningPorts={listeningPorts ?? []}
					portSharingControlsEnabled={
						entitlements.features.control_shared_ports.enabled
					}
					refetchSharedPorts={refetchSharedPorts}
				/>
			</PopoverContent>
		</Popover>
	);
};

const openPortSchema = (): Yup.AnyObjectSchema =>
	Yup.object({
		port: Yup.number().required().min(9).max(65535),
		share_level: Yup.string().required().oneOf(WorkspaceAppSharingLevels),
	});

interface PortForwardPopoverViewProps {
	host: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	template: Template;
	sharedPorts: readonly WorkspaceAgentPortShare[];
	listeningPorts: readonly WorkspaceAgentListeningPort[];
	portSharingControlsEnabled: boolean;
	refetchSharedPorts: () => void;
}

export const PortForwardPopoverView: FC<PortForwardPopoverViewProps> = ({
	host,
	workspace,
	agent,
	template,
	sharedPorts,
	listeningPorts,
	portSharingControlsEnabled,
	refetchSharedPorts,
}) => {
	const theme = useTheme();
	const [listeningPortProtocol, setListeningPortProtocol] = useState(
		getWorkspaceListeningPortsProtocol(workspace.id),
	);

	const upsertSharedPortMutation = useMutation({
		...upsertWorkspacePortShare(workspace.id),
		onSuccess: refetchSharedPorts,
	});

	const deleteSharedPortMutation = useMutation({
		...deleteWorkspacePortShare(workspace.id),
		onSuccess: refetchSharedPorts,
	});

	const {
		mutateAsync: upsertWorkspacePortShareForm,
		isPending: isSubmitting,
		error: submitError,
	} = useMutation({
		...upsertWorkspacePortShare(workspace.id),
		onSuccess: refetchSharedPorts,
	});

	const form = useFormik({
		initialValues: {
			agent_name: agent.name,
			port: "",
			protocol: "http",
			share_level: "authenticated",
		},
		validationSchema: openPortSchema(),
		onSubmit: async (values, { resetForm }) => {
			resetForm();
			await upsertWorkspacePortShareForm({
				agent_name: values.agent_name,
				port: Number(values.port),
				share_level: values.share_level as WorkspaceAgentPortShareLevel,
				protocol: values.protocol as WorkspaceAgentPortShareProtocol,
			});
		},
	});
	const getFieldHelpers = getFormHelpers(form, submitError);

	// filter out shared ports that are not from this agent
	const filteredSharedPorts = sharedPorts.filter(
		(port) => port.agent_name === agent.name,
	);
	// we don't want to show listening ports if it's a shared port
	const filteredListeningPorts = listeningPorts.filter((port) =>
		filteredSharedPorts.every((sharedPort) => sharedPort.port !== port.port),
	);
	// only disable the form if shared port controls are entitled and the template doesn't allow sharing ports
	const canSharePorts = !(
		portSharingControlsEnabled && template.max_port_share_level === "owner"
	);
	const canSharePortsPublic =
		canSharePorts && template.max_port_share_level === "public";
	const canSharePortsAuthenticated =
		canSharePorts &&
		(template.max_port_share_level === "authenticated" || canSharePortsPublic);

	const defaultShareLevel =
		template.max_port_share_level === "organization"
			? "organization"
			: "authenticated";

	const disabledPublicMenuItem = (
		<Tooltip>
			<TooltipTrigger asChild>
				{/* Tooltips don't work directly on disabled MenuItem components so you must wrap in div. */}
				<div>
					<MenuItem value="public" disabled>
						Public
					</MenuItem>
				</div>
			</TooltipTrigger>
			<TooltipContent disablePortal>
				This workspace template does not allow sharing ports publicly.
			</TooltipContent>
		</Tooltip>
	);

	const disabledAuthenticatedMenuItem = (
		<Tooltip>
			<TooltipTrigger asChild>
				{/* Tooltips don't work directly on disabled MenuItem components so you must wrap in div. */}
				<div>
					<MenuItem value="authenticated" disabled>
						Authenticated
					</MenuItem>
				</div>
			</TooltipTrigger>
			<TooltipContent disablePortal>
				This workspace template does not allow sharing ports outside of its
				organization.
			</TooltipContent>
		</Tooltip>
	);

	return (
		<>
			<div className="max-h-[320px] overflow-y-auto">
				<Stack direction="column" className="p-5">
					<Stack
						direction="row"
						justifyContent="space-between"
						alignItems="start"
					>
						<HelpTooltipTitle>Listening Ports</HelpTooltipTitle>
						<HelpTooltipLink
							href={docs("/admin/networking/port-forwarding#dashboard")}
						>
							Learn more
						</HelpTooltipLink>
					</Stack>
					<Stack direction="column" gap={1}>
						<HelpTooltipText css={{ color: theme.palette.text.secondary }}>
							The listening ports are exclusively accessible to you. Selecting
							HTTP/S will change the protocol for all listening ports.
						</HelpTooltipText>
						<Stack direction="row" gap={2} className="pb-2">
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
											workspace.id,
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
										workspace.name,
										workspace.owner_name,
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
								<Tooltip>
									<TooltipTrigger asChild>
										<Button type="submit" size="icon" variant="subtle">
											<ExternalLinkIcon />
											<span className="sr-only">Connect to port</span>
										</Button>
									</TooltipTrigger>
									<TooltipContent disablePortal>Connect to port</TooltipContent>
								</Tooltip>
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
							workspace.name,
							workspace.owner_name,
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
										<RadioIcon className="size-icon-sm" />
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
										<Tooltip>
											<TooltipTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													onClick={async () => {
														await upsertSharedPortMutation.mutateAsync({
															agent_name: agent.name,
															port: port.port,
															protocol: listeningPortProtocol,
															share_level: defaultShareLevel,
														});
													}}
												>
													<ShareIcon />
													<span className="sr-only">Share</span>
												</Button>
											</TooltipTrigger>
											<TooltipContent disablePortal>
												Share this port
											</TooltipContent>
										</Tooltip>
									)}
								</Stack>
							</Stack>
						);
					})}
				</Stack>
			</div>
			<div
				css={{
					borderTop: `1px solid ${theme.palette.divider}`,
				}}
				className="p-5"
			>
				<HelpTooltipTitle>Shared Ports</HelpTooltipTitle>
				<HelpTooltipText css={{ color: theme.palette.text.secondary }}>
					{canSharePorts
						? "Ports can be shared with organization members, other Coder users, or with the public."
						: "This workspace template does not allow sharing ports. Contact a template administrator to enable port sharing."}
				</HelpTooltipText>
				{canSharePorts && (
					<div>
						{filteredSharedPorts?.map((share) => {
							const url = portForwardURL(
								host,
								share.port,
								agent.name,
								workspace.name,
								workspace.owner_name,
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
											<LockOpenIcon className="size-icon-sm" />
										) : share.share_level === "organization" ? (
											<BuildingIcon className="size-icon-sm" />
										) : (
											<LockIcon className="size-icon-sm" />
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
												}}
											>
												<MenuItem value="organization">Organization</MenuItem>
												{canSharePortsAuthenticated ? (
													<MenuItem value="authenticated">
														Authenticated
													</MenuItem>
												) : (
													disabledAuthenticatedMenuItem
												)}
												{canSharePortsPublic ? (
													<MenuItem value="public">Public</MenuItem>
												) : (
													disabledPublicMenuItem
												)}
											</Select>
										</FormControl>
										<Button
											size="sm"
											variant="subtle"
											onClick={async () => {
												await deleteSharedPortMutation.mutateAsync({
													agent_name: agent.name,
													port: share.port,
												});
											}}
										>
											<XIcon
												css={{
													color: theme.palette.text.primary,
												}}
												className="size-3.5"
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
									<MenuItem value="organization">Organization</MenuItem>
									{canSharePortsAuthenticated ? (
										<MenuItem value="authenticated">Authenticated</MenuItem>
									) : (
										disabledAuthenticatedMenuItem
									)}
									{canSharePortsPublic ? (
										<MenuItem value="public">Public</MenuItem>
									) : (
										disabledPublicMenuItem
									)}
								</TextField>
								<Button type="submit" disabled={!form.isValid || isSubmitting}>
									<Spinner loading={isSubmitting} />
									Share Port
								</Button>
							</Stack>
						</form>
					</div>
				)}
			</div>
		</>
	);
};

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

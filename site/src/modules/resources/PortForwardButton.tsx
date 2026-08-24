import { useFormik } from "formik";
import {
	BuildingIcon,
	ExternalLinkIcon,
	LockIcon,
	LockOpenIcon,
	RadioIcon,
	ShareIcon,
	XIcon,
} from "lucide-react";
import { type FC, useId, useState } from "react";
import { useMutation } from "react-query";
import * as Yup from "yup";
import {
	deleteWorkspacePortShare,
	upsertWorkspacePortShare,
} from "#/api/queries/workspaceportsharing";
import {
	type Template,
	type Workspace,
	type WorkspaceAgent,
	type WorkspaceAgentListeningPort,
	type WorkspaceAgentPortShare,
	type WorkspaceAgentPortShareLevel,
	WorkspaceAgentPortShareLevels,
	type WorkspaceAgentPortShareProtocol,
} from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import {
	HelpPopoverLink,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { usePortsData } from "#/modules/resources/usePortsData";
import { docs } from "#/utils/docs";
import { getFormHelpers } from "#/utils/formUtils";
import {
	getWorkspaceListeningPortsProtocol,
	portForwardURL,
	saveWorkspaceListeningPortsProtocol,
} from "#/utils/portForward";

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

	const { listeningPorts, sharedPorts, refetchSharedPorts } = usePortsData(
		workspace,
		agent,
		agent.status === "connected",
	);

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button disabled={!listeningPorts} size="sm" variant="subtle">
					<Spinner loading={!listeningPorts}>
						<span className="text-xs font-medium h-5 min-w-5 px-1 rounded-full flex items-center justify-center bg-surface-tertiary">
							{listeningPorts?.length}
						</span>
					</Spinner>
					Open ports
					<ChevronDownIcon />
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

type OpenPortFormValues = {
	agent_name: string;
	port: string;
	protocol: WorkspaceAgentPortShareProtocol;
	share_level: WorkspaceAgentPortShareLevel;
};

const openPortSchema = () =>
	Yup.object({
		port: Yup.number().required().min(9).max(65535),
		share_level: Yup.string().required().oneOf(WorkspaceAgentPortShareLevels),
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

const isPortShareProtocol = (
	value: string,
): value is WorkspaceAgentPortShareProtocol =>
	value === "http" || value === "https";

const isPortShareLevel = (
	value: string,
): value is WorkspaceAgentPortShareLevel =>
	WorkspaceAgentPortShareLevels.some((level) => level === value);

const isListeningPortProtocol = (value: string): value is "http" | "https" =>
	value === "http" || value === "https";

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
	const [listeningPortProtocol, setListeningPortProtocol] = useState(
		getWorkspaceListeningPortsProtocol(workspace.id),
	);
	const protocolFieldId = useId();
	const shareLevelFieldId = useId();

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

	const form = useFormik<OpenPortFormValues>({
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
				share_level: values.share_level,
				protocol: values.protocol,
			});
		},
	});
	const getFieldHelpers = getFormHelpers(form, submitError);
	const protocolField = getFieldHelpers("protocol");
	const shareLevelField = getFieldHelpers("share_level");

	// usePortsData already filters shared ports down to this agent, so only
	// hide listening ports that are also shared.
	const filteredListeningPorts = listeningPorts.filter((port) =>
		sharedPorts.every((sharedPort) => sharedPort.port !== port.port),
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

	const renderShareLevelOptions = () => (
		<>
			<SelectItem value="organization">Organization</SelectItem>
			{canSharePortsAuthenticated ? (
				<SelectItem value="authenticated">Authenticated</SelectItem>
			) : (
				<SelectItem
					value="authenticated"
					disabled
					title="This workspace template does not allow sharing ports outside of its organization."
				>
					Authenticated
				</SelectItem>
			)}
			{canSharePortsPublic ? (
				<SelectItem value="public">Public</SelectItem>
			) : (
				<SelectItem
					value="public"
					disabled
					title="This workspace template does not allow sharing ports publicly."
				>
					Public
				</SelectItem>
			)}
		</>
	);

	return (
		<>
			<div className="max-h-80 overflow-y-auto">
				<div className="flex flex-col p-5">
					<div className="flex flex-row justify-between items-start">
						<HelpPopoverTitle>Listening Ports</HelpPopoverTitle>
						<HelpPopoverLink
							href={docs("/admin/networking/port-forwarding#dashboard")}
						>
							Learn more
						</HelpPopoverLink>
					</div>
					<div className="flex flex-col gap-1">
						<HelpPopoverText>
							The listening ports are exclusively accessible to you. Selecting
							HTTP/S will change the protocol for all listening ports.
						</HelpPopoverText>
						<div className="flex flex-row gap-2 pb-2">
							<Select
								value={listeningPortProtocol}
								onValueChange={(value) => {
									if (!isListeningPortProtocol(value)) {
										return;
									}
									setListeningPortProtocol(value);
									saveWorkspaceListeningPortsProtocol(workspace.id, value);
								}}
							>
								<SelectTrigger
									aria-label="Listening port protocol"
									className="h-[34px] min-w-[100px] mt-2 w-auto"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="http">HTTP</SelectItem>
									<SelectItem value="https">HTTPS</SelectItem>
								</SelectContent>
							</Select>
							<form
								className="mt-2 flex w-full items-center rounded border border-solid border-border focus-within:border-content-link"
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
									className="block h-[34px] w-full border-0 bg-transparent px-3 text-sm text-content-primary outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
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
						</div>
					</div>
					{filteredListeningPorts.length === 0 && (
						<HelpPopoverText className="text-content-secondary pt-5 pb-2.5 text-center">
							No open ports were detected.
						</HelpPopoverText>
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
							<div
								key={port.port}
								className="flex flex-row items-center justify-between"
							>
								<div className="flex flex-row gap-3">
									<a
										className="flex min-w-20 items-center gap-2 py-2 text-sm font-medium text-content-primary no-underline hover:underline"
										href={url}
										target="_blank"
										rel="noreferrer"
									>
										<RadioIcon className="size-icon-sm" />
										{port.port}
									</a>
									<a
										className="flex min-w-20 items-center gap-2 py-2 text-sm font-medium text-content-primary no-underline hover:underline"
										href={url}
										target="_blank"
										rel="noreferrer"
									>
										{label}
									</a>
								</div>
								<div className="flex flex-row gap-2 justify-end items-center">
									{canSharePorts && (
										<Tooltip>
											<TooltipTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													onClick={() => {
														upsertSharedPortMutation.mutate({
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
								</div>
							</div>
						);
					})}
				</div>
			</div>
			<div className="p-5 border-0 border-t border-solid border-border">
				<HelpPopoverTitle>Shared Ports</HelpPopoverTitle>
				<HelpPopoverText>
					{canSharePorts
						? "Ports can be shared with organization members, other Coder users, or with the public."
						: "This workspace template does not allow sharing ports. Contact a template administrator to enable port sharing."}
				</HelpPopoverText>
				{canSharePorts && (
					<div>
						{sharedPorts.map((share) => {
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
								<div
									key={share.port}
									className="flex flex-row justify-between items-center"
								>
									<a
										className="flex min-w-20 items-center gap-2 py-2 text-sm font-medium text-content-primary no-underline hover:underline"
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
									</a>
									<Select
										value={share.protocol}
										onValueChange={(value) => {
											if (!isPortShareProtocol(value)) {
												return;
											}
											upsertSharedPortMutation.mutate({
												agent_name: agent.name,
												port: share.port,
												protocol: value,
												share_level: share.share_level,
											});
										}}
									>
										<SelectTrigger
											aria-label={`Protocol for port ${share.port}`}
											className="h-8 min-w-[5.625rem] w-auto border-0 shadow-none focus:ring-0"
										>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="http">HTTP</SelectItem>
											<SelectItem value="https">HTTPS</SelectItem>
										</SelectContent>
									</Select>

									<div className="flex flex-row justify-end">
										<Select
											value={share.share_level}
											onValueChange={(value) => {
												if (!isPortShareLevel(value)) {
													return;
												}
												upsertSharedPortMutation.mutate({
													agent_name: agent.name,
													port: share.port,
													protocol: share.protocol,
													share_level: value,
												});
											}}
										>
											<SelectTrigger
												aria-label={`Sharing level for port ${share.port}`}
												className="h-8 min-w-[8.75rem] w-auto border-0 shadow-none focus:ring-0"
											>
												<SelectValue />
											</SelectTrigger>
											<SelectContent>{renderShareLevelOptions()}</SelectContent>
										</Select>
										<Button
											size="icon"
											variant="subtle"
											aria-label="Delete shared port"
											onClick={() => {
												deleteSharedPortMutation.mutate({
													agent_name: agent.name,
													port: share.port,
												});
											}}
										>
											<XIcon />
										</Button>
									</div>
								</div>
							);
						})}
						<form onSubmit={form.handleSubmit}>
							<div className="mt-4 flex flex-col gap-4 justify-end">
								<FormField
									field={getFieldHelpers("port")}
									label="Port"
									disabled={isSubmitting}
									type="number"
									min={9}
									max={65535}
								/>
								<div className="flex flex-col gap-2">
									<Label htmlFor={protocolFieldId}>Protocol</Label>
									<Select
										value={form.values.protocol}
										onValueChange={(value) => {
											if (!isPortShareProtocol(value)) {
												return;
											}
											void form.setFieldValue("protocol", value);
										}}
										disabled={isSubmitting}
									>
										<SelectTrigger
											id={protocolFieldId}
											aria-invalid={protocolField.error}
											className={
												protocolField.error
													? "border-border-destructive"
													: undefined
											}
										>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="http">HTTP</SelectItem>
											<SelectItem value="https">HTTPS</SelectItem>
										</SelectContent>
									</Select>
								</div>
								<div className="flex flex-col gap-2">
									<Label htmlFor={shareLevelFieldId}>Sharing Level</Label>
									<Select
										value={form.values.share_level}
										onValueChange={(value) => {
											if (!isPortShareLevel(value)) {
												return;
											}
											void form.setFieldValue("share_level", value);
										}}
										disabled={isSubmitting}
									>
										<SelectTrigger
											id={shareLevelFieldId}
											aria-label="Sharing Level"
											aria-invalid={shareLevelField.error}
											className={
												shareLevelField.error
													? "border-border-destructive"
													: undefined
											}
										>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>{renderShareLevelOptions()}</SelectContent>
									</Select>
								</div>
								<Button type="submit" disabled={!form.isValid || isSubmitting}>
									<Spinner loading={isSubmitting} />
									Share Port
								</Button>
							</div>
						</form>
					</div>
				)}
			</div>
		</>
	);
};

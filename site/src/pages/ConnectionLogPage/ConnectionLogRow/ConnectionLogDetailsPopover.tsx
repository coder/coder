import { InfoIcon } from "lucide-react";
import { useRef, useState } from "react";
import { Link as RouterLink } from "react-router";
import userAgentParser from "ua-parser-js";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Link } from "#/components/Link/Link";
import {
	type CompatibleConnectionLog,
	type ConnectionDuration,
	ConnectionLogDurationDetails,
} from "./ConnectionLogDuration";

type ConnectionLogDetailsPopoverProps = {
	connectionLog: CompatibleConnectionLog;
	duration: ConnectionDuration | undefined;
};

/** Shows connection metadata and workspace-scoped correlation actions. */
export const ConnectionLogDetailsPopover: React.FC<
	ConnectionLogDetailsPopoverProps
> = ({ connectionLog, duration }) => {
	const [isOpen, setIsOpen] = useState(false);
	const contentRef = useRef<HTMLDivElement>(null);
	const userAgent = connectionLog.web_info?.user_agent
		? userAgentParser(connectionLog.web_info.user_agent)
		: undefined;
	const sshInfo = connectionLog.ssh_info;

	return (
		<HelpPopover open={isOpen} onOpenChange={setIsOpen}>
			<HelpPopoverIconTrigger aria-label="Connection details">
				<InfoIcon />
			</HelpPopoverIconTrigger>
			<HelpPopoverContent
				ref={contentRef}
				side="bottom"
				align="end"
				className="flex flex-col gap-2"
				// Focus the panel to avoid opening the copy tooltip.
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					contentRef.current?.focus();
				}}
			>
				{connectionLog.ip && (
					<ConnectionLogDetail title="IP:">
						{connectionLog.ip}
					</ConnectionLogDetail>
				)}
				{userAgent?.os.name && (
					<ConnectionLogDetail title="OS:">
						{userAgent.os.name}
					</ConnectionLogDetail>
				)}
				{userAgent?.browser.name && (
					<ConnectionLogDetail title="Browser:">
						{userAgent.browser.name} {userAgent.browser.version}
					</ConnectionLogDetail>
				)}
				{connectionLog.organization && (
					<ConnectionLogDetail title="Organization:">
						<Link asChild size="sm" showExternalIcon={false} className="px-0">
							<RouterLink
								to={`/organizations/${connectionLog.organization.name}`}
							>
								{connectionLog.organization.display_name ||
									connectionLog.organization.name}
							</RouterLink>
						</Link>
					</ConnectionLogDetail>
				)}
				{sshInfo?.disconnect_reason && (
					<ConnectionLogDetail title="Close Reason:">
						{sshInfo.disconnect_reason}
					</ConnectionLogDetail>
				)}
				{duration && <ConnectionLogDurationDetails duration={duration} />}
				{sshInfo?.client_session_id && (
					<ClientSessionDetail
						clientSessionId={sshInfo.client_session_id}
						workspaceId={connectionLog.workspace_id}
						onNavigate={() => setIsOpen(false)}
					/>
				)}
			</HelpPopoverContent>
		</HelpPopover>
	);
};

type ClientSessionDetailProps = {
	clientSessionId: string;
	workspaceId: string;
	onNavigate: () => void;
};

const ClientSessionDetail: React.FC<ClientSessionDetailProps> = ({
	clientSessionId,
	workspaceId,
	onNavigate,
}) => {
	const relatedConnectionsParams = new URLSearchParams({
		[useFilterParamsKey]: `client_session_id:${clientSessionId} workspace_id:${workspaceId}`,
	});

	return (
		<ConnectionLogDetail title="Client Session ID:">
			<div className="flex flex-row items-center gap-1">
				<code className="break-all">{clientSessionId}</code>
				<CopyButton text={clientSessionId} label="Copy client session ID" />
			</div>
			<p className="m-0">
				Finds connections with this client session ID. Clients may reuse this
				ID.
			</p>
			<Link asChild size="sm" showExternalIcon={false} className="px-0">
				<RouterLink
					to={`/connectionlog?${relatedConnectionsParams.toString()}`}
					onClick={onNavigate}
				>
					View related connections
				</RouterLink>
			</Link>
		</ConnectionLogDetail>
	);
};

type ConnectionLogDetailProps = {
	title: string;
	children: React.ReactNode;
};

const ConnectionLogDetail: React.FC<ConnectionLogDetailProps> = ({
	title,
	children,
}) => (
	<div>
		<HelpPopoverTitle className="mb-0">{title}</HelpPopoverTitle>
		<div className="text-content-secondary text-xs">{children}</div>
	</div>
);

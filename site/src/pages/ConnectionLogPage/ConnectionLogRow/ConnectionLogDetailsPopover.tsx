import { InfoIcon } from "lucide-react";
import { useRef, useState } from "react";
import { Link as RouterLink } from "react-router";
import userAgentParser from "ua-parser-js";
import type { ConnectionLog } from "#/api/typesGenerated";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import { HelpPopoverIconTrigger } from "#/components/HelpPopover/HelpPopover";
import { Link } from "#/components/Link/Link";
import { Popover, PopoverContent } from "#/components/Popover/Popover";

type ConnectionLogDetailsPopoverProps = {
	connectionLog: ConnectionLog;
};

/** Shows connection metadata and links to related connections. */
export const ConnectionLogDetailsPopover: React.FC<
	ConnectionLogDetailsPopoverProps
> = ({ connectionLog }) => {
	const [isOpen, setIsOpen] = useState(false);
	const contentRef = useRef<HTMLDivElement>(null);
	const { ip, organization, ssh_info: sshInfo, web_info } = connectionLog;
	const userAgent = web_info?.user_agent
		? userAgentParser(web_info.user_agent)
		: undefined;
	const clientSessionId = sshInfo?.client_session_id;

	return (
		<Popover open={isOpen} onOpenChange={setIsOpen}>
			<HelpPopoverIconTrigger aria-label="Connection details">
				<InfoIcon />
			</HelpPopoverIconTrigger>
			<PopoverContent
				ref={contentRef}
				align="end"
				className="w-auto max-w-72 p-3 flex flex-col gap-4 text-xs font-normal"
				// Focus the panel so the copy button's tooltip doesn't open.
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					contentRef.current?.focus();
				}}
			>
				<dl className="m-0 flex flex-col gap-2">
					{ip && <Detail title="IP">{ip}</Detail>}
					{userAgent?.os.name && (
						<Detail title="OS">{userAgent.os.name}</Detail>
					)}
					{userAgent?.browser.name && (
						<Detail title="Browser">
							{userAgent.browser.name} {userAgent.browser.version}
						</Detail>
					)}
					{organization && (
						<Detail title="Organization">
							<Link
								asChild
								size="sm"
								showExternalIcon={false}
								className="px-0 leading-4"
							>
								<RouterLink to={`/organizations/${organization.name}`}>
									{organization.display_name || organization.name}
								</RouterLink>
							</Link>
						</Detail>
					)}
					{sshInfo?.disconnect_reason && (
						<Detail title="Close reason">{sshInfo.disconnect_reason}</Detail>
					)}
					{clientSessionId && (
						<Detail title="Client session ID">
							<div className="flex flex-row items-center gap-1">
								<span className="min-w-0 truncate">{clientSessionId}</span>
								<CopyButton
									text={clientSessionId}
									label="Copy client session ID"
									className="size-6 shrink-0 -my-1"
								/>
							</div>
						</Detail>
					)}
				</dl>
				{clientSessionId && (
					<Link
						asChild
						size="sm"
						showExternalIcon={false}
						className="px-0 mr-auto"
					>
						<RouterLink
							to={`/connectionlog?${new URLSearchParams({
								[useFilterParamsKey]: `client_session_id:${clientSessionId} workspace_id:${connectionLog.workspace_id}`,
							})}`}
							onClick={() => setIsOpen(false)}
						>
							View related connections
						</RouterLink>
					</Link>
				)}
			</PopoverContent>
		</Popover>
	);
};

type DetailProps = {
	title: string;
	children: React.ReactNode;
};

const Detail: React.FC<DetailProps> = ({ title, children }) => (
	<div>
		<dt className="text-content-secondary">{title}</dt>
		<dd className="m-0 text-content-primary">{children}</dd>
	</div>
);

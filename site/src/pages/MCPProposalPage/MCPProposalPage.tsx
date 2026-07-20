import {
	ChevronDownIcon,
	CopyIcon,
	GlobeIcon,
	InfoIcon,
	KeyRoundIcon,
	LockKeyholeIcon,
	UserRoundIcon,
} from "lucide-react";
import { type FC, type ReactNode, useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useParams } from "react-router";
import { getErrorStatus } from "#/api/errors";
import {
	acceptMCPServerProposal,
	mcpServerProposal,
	rejectMCPServerProposal,
} from "#/api/queries/mcpServerProposals";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { CopyableValue } from "#/components/CopyableValue/CopyableValue";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import { Markdown } from "#/components/Markdown/Markdown";
import { Spinner } from "#/components/Spinner/Spinner";
import { Field } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerFormFieldPrimitives";
import { MCPServerIcon } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerIcon";
import {
	AUTH_TYPE_LABELS,
	TRANSPORT_OPTIONS,
} from "#/pages/AISettingsPage/MCPServersPage/components/mcpServerFormLogic";
import { pageTitle } from "#/utils/page";
import { relativeTime } from "#/utils/time";

interface MCPProposalPageProps {
	/**
	 * Performs the full-page navigation to the OAuth2 connect URL after a
	 * proposal is accepted. Injectable so stories can assert the redirect
	 * without navigating away from the test page.
	 */
	redirectToConnectUrl?: (url: string) => void;
}

const MCPProposalPage: FC<MCPProposalPageProps> = ({
	redirectToConnectUrl = (url) => location.assign(url),
}) => {
	const { proposal: proposalId } = useParams() as { proposal: string };
	const queryClient = useQueryClient();
	const [redirecting, setRedirecting] = useState(false);
	const [inputValues, setInputValues] =
		useState<TypesGen.AcceptMCPServerProposalRequest>({ values: {} });

	const proposalQuery = useQuery({
		...mcpServerProposal(proposalId),
		// Expired and forbidden proposals never recover on retry, and the
		// page has an explicit retry affordance for transient failures.
		retry: false,
	});
	const acceptMutation = useMutation(
		acceptMCPServerProposal(queryClient, proposalId),
	);
	const rejectMutation = useMutation(rejectMCPServerProposal(proposalId));

	const handleAccept = () => {
		acceptMutation.mutate(inputValues, {
			onSuccess: (res) => {
				if (res.connect_url && !res.authenticated) {
					setRedirecting(true);
					redirectToConnectUrl(res.connect_url);
				}
			},
		});
	};

	return (
		<>
			<title>{pageTitle("Review MCP server proposal")}</title>
			<Breadcrumb>
				<BreadcrumbList className="m-0 mb-[14px] gap-2 p-0 text-[13px] font-medium leading-none">
					<BreadcrumbItem>
						<BreadcrumbLink
							asChild
							className="!no-underline hover:!no-underline"
						>
							<Link to="/agents/settings/mcp-servers">
								Personal MCP servers
							</Link>
						</BreadcrumbLink>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbPage className="text-content-primary">
							Review proposal
						</BreadcrumbPage>
					</BreadcrumbItem>
				</BreadcrumbList>
			</Breadcrumb>
			<div className="mb-[30px]">
				<h1 className="m-0 text-[28px] font-bold leading-[1.15] tracking-[-0.01em] text-content-primary">
					Review MCP server proposal
				</h1>
				<p className="m-0 mt-2 max-w-[41.25rem] text-sm font-normal leading-[1.6] text-content-secondary">
					Approve to use this MCP server in Coder Agents.
				</p>
			</div>
			<MCPProposalPageContent
				proposalQuery={proposalQuery}
				acceptMutation={acceptMutation}
				rejectMutation={rejectMutation}
				redirecting={redirecting}
				inputValues={inputValues}
				onInputValuesChange={setInputValues}
				onAccept={handleAccept}
				onReject={() => rejectMutation.mutate()}
			/>
		</>
	);
};

interface MCPProposalPageContentProps {
	proposalQuery: ReturnType<typeof useQuery<TypesGen.MCPServerProposal>>;
	acceptMutation: ReturnType<
		typeof useMutation<
			TypesGen.AcceptMCPServerProposalResponse,
			unknown,
			TypesGen.AcceptMCPServerProposalRequest
		>
	>;
	rejectMutation: ReturnType<typeof useMutation<void, unknown, void>>;
	redirecting: boolean;
	inputValues: TypesGen.AcceptMCPServerProposalRequest;
	onInputValuesChange: (
		values: TypesGen.AcceptMCPServerProposalRequest,
	) => void;
	onAccept: () => void;
	onReject: () => void;
}

const MCPProposalPageContent: FC<MCPProposalPageContentProps> = ({
	proposalQuery,
	acceptMutation,
	rejectMutation,
	redirecting,
	inputValues,
	onInputValuesChange,
	onAccept,
	onReject,
}) => {
	if (rejectMutation.isSuccess) {
		return (
			<EmptyState
				message="Proposal rejected"
				description="The MCP server will not be created. You can close this page."
			/>
		);
	}

	if (redirecting) {
		return (
			<EmptyState
				message="Redirecting to authentication"
				description="Taking you to the MCP server's sign-in flow to connect your account."
				cta={<Loader />}
			/>
		);
	}

	if (acceptMutation.isSuccess) {
		return <AcceptedState connectUrl={acceptMutation.data.connect_url} />;
	}

	if (proposalQuery.isLoading) {
		return <Loader />;
	}

	if (proposalQuery.error !== null || !proposalQuery.data) {
		return (
			<ProposalError
				error={proposalQuery.error}
				onRetry={() => proposalQuery.refetch()}
			/>
		);
	}

	const proposal = proposalQuery.data;

	if (proposal.status === "accepted") {
		return (
			<AcceptedState
				connectUrl={proposal.authenticated ? undefined : proposal.connect_url}
			/>
		);
	}

	if (proposal.status === "rejected") {
		return (
			<EmptyState
				message="Proposal rejected"
				description="The MCP server will not be created. You can close this page."
			/>
		);
	}

	const canAccept = requiredInputsFilled(proposal.required_inputs, inputValues);

	return (
		<div className="overflow-hidden rounded-xl border border-solid border-border-default bg-surface-primary">
			<header className="flex items-center gap-3.5 border-0 border-b border-solid border-border-default px-[22px] py-[18px]">
				<MCPServerIcon
					iconUrl={proposal.icon_url ?? ""}
					name={proposal.display_name}
					className="size-11 rounded-[10px]"
				/>
				<div className="min-w-0">
					<h2 className="m-0 truncate text-base font-semibold leading-[1.1] tracking-[-0.01em] text-content-primary">
						{proposal.display_name}
					</h2>
					<p className="m-0 mt-1 text-xs leading-[1.3] text-content-secondary">
						MCP server proposal · requested {relativeTime(proposal.created_at)}
					</p>
				</div>
				<Badge
					variant="warning"
					size="sm"
					className="ml-auto hidden h-6 shrink-0 rounded-full border-highlight-orange bg-transparent px-2.5 py-0 text-[11px] font-semibold leading-none text-highlight-orange shadow-none sm:inline-flex"
				>
					<span className="size-1.5 rounded-full bg-current" />
					Pending review
				</Badge>
			</header>

			<div className="grid lg:grid-cols-[minmax(0,1fr)_21.25rem]">
				<div className="flex min-w-0 flex-col gap-[22px] p-6 lg:border-0 lg:border-r lg:border-solid lg:border-border-default">
					{proposal.create_disabled && (
						<Alert severity="warning">
							This MCP server will be created in a disabled state.
						</Alert>
					)}

					<ProposalDetails proposal={proposal} />

					{proposal.oauth2_redirect_uri && (
						<OAuth2RedirectURI uri={proposal.oauth2_redirect_uri} />
					)}

					<ProposalInputFields
						instructions={proposal.instructions}
						inputs={proposal.required_inputs}
						values={inputValues}
						onChange={onInputValuesChange}
						disabled={acceptMutation.isPending || rejectMutation.isPending}
					/>

					{acceptMutation.error !== null && (
						<ErrorAlert error={acceptMutation.error} />
					)}
					{rejectMutation.error !== null && (
						<ErrorAlert error={rejectMutation.error} />
					)}
				</div>

				<aside
					aria-label="Proposal summary and actions"
					className="flex min-w-0 flex-col border-0 border-t border-solid border-border-default px-[22px] py-6 lg:border-t-0"
				>
					<ProposalSummary proposal={proposal} />

					<div className="mt-8 pt-[22px] lg:mt-auto">
						<Button
							className="w-full"
							onClick={onAccept}
							disabled={
								!canAccept ||
								acceptMutation.isPending ||
								rejectMutation.isPending
							}
						>
							<Spinner loading={acceptMutation.isPending} />
							Accept &amp; create server
						</Button>
						<Button
							className="mt-2.5 w-full"
							variant="destructive"
							onClick={onReject}
							disabled={acceptMutation.isPending || rejectMutation.isPending}
						>
							<Spinner loading={rejectMutation.isPending} />
							Reject
						</Button>
						{!canAccept && (
							<p className="m-0 mt-2.5 text-center text-[11px] font-medium leading-[1.4] text-content-secondary">
								Enter all required values to enable approval.
							</p>
						)}
					</div>
				</aside>
			</div>
		</div>
	);
};

const requiredInputsFilled = (
	inputs: readonly TypesGen.MCPServerProposalInput[],
	values: TypesGen.AcceptMCPServerProposalRequest,
): boolean => {
	return inputs.every((input) => values.values?.[input.field]?.trim());
};

interface ProposalInputFieldsProps {
	instructions?: string;
	inputs: readonly TypesGen.MCPServerProposalInput[];
	values: TypesGen.AcceptMCPServerProposalRequest;
	onChange: (values: TypesGen.AcceptMCPServerProposalRequest) => void;
	disabled: boolean;
}

const ProposalInputFields: FC<ProposalInputFieldsProps> = ({
	instructions,
	inputs,
	values,
	onChange,
	disabled,
}) => {
	const formId = useId();
	const hasInputFields = inputs.length > 0;

	if (!hasInputFields && !instructions) {
		return null;
	}

	if (!hasInputFields && instructions) {
		return <ProposalInstructions instructions={instructions} standalone />;
	}

	return (
		<section
			aria-label="Required configuration"
			className="flex flex-col gap-[18px] border-0 border-t border-solid border-border-default pt-[22px]"
		>
			<div>
				<h2 className="m-0 flex items-center gap-2 text-sm font-semibold text-content-primary">
					<LockKeyholeIcon className="size-[15px] text-content-warning" />
					Configuration required
				</h2>
				<p className="m-0 mt-1 text-[13px] font-normal leading-[1.5] text-content-secondary">
					Sent directly to Coder to create the server. Never shared with the
					agent.
				</p>
			</div>

			{instructions && <ProposalInstructions instructions={instructions} />}

			<div className="grid items-start gap-[14px]">
				{inputs.map((input, index) => (
					<ProposalInputField
						key={input.field}
						id={`${formId}-input-${index}`}
						label={input.label}
						placeholder={input.placeholder}
						sensitive={input.sensitive}
						value={values.values?.[input.field] ?? ""}
						onChange={(value) =>
							onChange({
								...values,
								values: {
									...values.values,
									[input.field]: value,
								},
							})
						}
						disabled={disabled}
					/>
				))}
			</div>
		</section>
	);
};

const OAuth2RedirectURI: FC<{ uri: string }> = ({ uri }) => {
	return (
		<section
			aria-label="OAuth2 redirect URI"
			className="flex flex-col gap-2 border-0 border-t border-solid border-border-default pt-[22px]"
		>
			<h2 className="m-0 text-sm font-semibold text-content-primary">
				OAuth2 redirect URI
			</h2>
			<p className="m-0 text-[13px] font-normal leading-[1.5] text-content-secondary">
				Register this URI with the OAuth2 provider when creating the
				application.
			</p>
			<CopyableValue
				value={uri}
				className="break-all rounded-[10px] border border-solid border-border-default bg-surface-primary px-3.5 py-3 font-mono text-[13px] leading-[1.4] text-content-primary"
			>
				{uri} <CopyIcon className="inline size-icon-xs align-text-bottom" />
			</CopyableValue>
		</section>
	);
};

const ProposalInstructions: FC<{
	instructions: string;
	standalone?: boolean;
}> = ({ instructions, standalone = false }) => {
	const [open, setOpen] = useState(false);

	if (standalone) {
		return (
			<section aria-label="Instructions" className="flex flex-col gap-4">
				<h2 className="m-0 text-sm font-semibold text-content-primary">
					Setup instructions
				</h2>
				<ProposalAuthorNotice />
				<Markdown className={proposalInstructionsMarkdownClassName}>
					{instructions}
				</Markdown>
			</section>
		);
	}

	return (
		<Collapsible
			open={open}
			onOpenChange={setOpen}
			className="overflow-hidden rounded-[10px] border border-solid border-border-default bg-surface-primary"
		>
			<CollapsibleTrigger asChild>
				<Button
					variant="subtle"
					className="h-auto w-full !justify-start rounded-none px-[18px] pb-2.5 pt-3.5 text-[13px] font-semibold leading-[1.3] text-content-primary"
				>
					<span className="flex-1 text-left">Setup instructions</span>
					<ChevronDownIcon
						className={`size-icon-sm text-content-secondary transition-transform ${open ? "rotate-180" : ""}`}
					/>
				</Button>
			</CollapsibleTrigger>
			{!open && (
				<div className="relative px-[18px] pb-4">
					<div
						inert
						aria-hidden="true"
						className="pointer-events-none max-h-24 select-none overflow-hidden"
					>
						<ProposalAuthorNotice />
						<Markdown
							className={`mt-4 ${proposalInstructionsMarkdownClassName}`}
						>
							{instructions}
						</Markdown>
					</div>
					<div className="absolute inset-x-0 bottom-0 flex h-16 items-end justify-center bg-gradient-to-b from-transparent to-surface-primary pb-2">
						<CollapsibleTrigger asChild>
							<Button
								variant="subtle"
								size="xs"
								className="!min-w-0 !text-xs text-content-link"
							>
								Show all
							</Button>
						</CollapsibleTrigger>
					</div>
				</div>
			)}
			<CollapsibleContent
				role="region"
				aria-label="Instructions"
				className="px-[18px] pb-4"
			>
				<ProposalAuthorNotice />
				<Markdown className={`mt-4 ${proposalInstructionsMarkdownClassName}`}>
					{instructions}
				</Markdown>
			</CollapsibleContent>
		</Collapsible>
	);
};

const proposalInstructionsMarkdownClassName = `
	!text-[13px] !leading-5 text-content-secondary
	[&_h1]:!mb-2 [&_h1]:!mt-4 [&_h1]:!text-base [&_h1]:!leading-6
	[&_h2]:!mb-2 [&_h2]:!mt-4 [&_h2]:!text-base [&_h2]:!leading-6
	[&_h3]:!mb-2 [&_h3]:!mt-4 [&_h3]:!text-sm [&_h3]:!leading-5
	[&_h4]:!mb-2 [&_h4]:!mt-4 [&_h4]:!text-sm [&_h4]:!leading-5
	[&_h5]:!mb-2 [&_h5]:!mt-4 [&_h5]:!text-sm [&_h5]:!leading-5
	[&_h6]:!mb-2 [&_h6]:!mt-4 [&_h6]:!text-sm [&_h6]:!leading-5
	[&_p]:!mb-2 [&_ol]:!mb-2 [&_ol]:!gap-1.5 [&_ul]:!mb-2 [&_ul]:!gap-1.5
	[&_code]:!text-xs
`;

const ProposalAuthorNotice: FC = () => (
	<div className="flex items-start gap-1.5 text-[11px] font-normal leading-[1.4] text-content-secondary">
		<InfoIcon className="mt-0.5 size-3 shrink-0" />
		<span>
			Written by the agent. Verify links and commands before following them.
		</span>
	</div>
);

const ProposalInputField: FC<{
	id: string;
	label: string;
	placeholder: string;
	sensitive: boolean;
	value: string;
	onChange: (value: string) => void;
	disabled: boolean;
}> = ({ id, label, placeholder, sensitive, value, onChange, disabled }) => (
	<Field
		label={label}
		htmlFor={id}
		required
		className="[&>label]:text-xs [&>label]:font-medium"
	>
		<Input
			id={id}
			type={sensitive ? "password" : "text"}
			autoComplete="off"
			data-1p-ignore
			data-lpignore="true"
			data-form-type="other"
			data-bwignore
			className="h-[38px] text-[13px] font-medium leading-none"
			placeholder={placeholder}
			value={value}
			onChange={(event) => onChange(event.target.value)}
			disabled={disabled}
		/>
	</Field>
);

const AcceptedState: FC<{ connectUrl?: string }> = ({ connectUrl }) => {
	return (
		<EmptyState
			message="MCP server created"
			description={
				connectUrl
					? "The server was created and enabled for the chat. Connect your account to finish setting it up."
					: "The server was created and enabled for the chat."
			}
			cta={
				connectUrl ? (
					<Button asChild>
						<a href={connectUrl}>Connect your account</a>
					</Button>
				) : undefined
			}
		/>
	);
};

interface ProposalErrorProps {
	error: unknown;
	onRetry: () => void;
}

const ProposalError: FC<ProposalErrorProps> = ({ error, onRetry }) => {
	const status = getErrorStatus(error);

	if (status === 404) {
		return (
			<EmptyState
				message="Proposal unavailable"
				description="This MCP server proposal has expired or was already handled."
			/>
		);
	}

	if (status === 403) {
		return (
			<EmptyState
				message="Not authorized"
				description="Another user must authorize this MCP server proposal."
			/>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<ErrorAlert error={error} />
			<div>
				<Button variant="outline" onClick={onRetry}>
					Retry
				</Button>
			</div>
		</div>
	);
};

const transportLabel = (transport: string) =>
	TRANSPORT_OPTIONS.find((option) => option.value === transport)?.label ??
	transport;

const ProposalDetails: FC<{ proposal: TypesGen.MCPServerProposal }> = ({
	proposal,
}) => {
	const providedAuthMaterial = [
		proposal.has_oauth2_client_credentials && "OAuth2 client credentials",
		proposal.has_api_key && "API key",
		proposal.has_custom_headers && "Custom headers",
	].filter((material): material is string => typeof material === "string");

	return (
		<section
			aria-label="Proposed configuration"
			className="grid gap-x-6 gap-y-[18px] sm:grid-cols-2"
		>
			<DetailRow label="Display name">{proposal.display_name}</DetailRow>
			<DetailRow
				label="Slug"
				valueClassName="font-mono text-[13px] leading-[1.3]"
			>
				{proposal.slug}
			</DetailRow>
			<DetailRow
				label="URL"
				className="sm:col-span-2"
				valueClassName="font-mono text-[13px] leading-[1.4]"
			>
				{proposal.url}
			</DetailRow>
			<DetailRow label="Transport">
				{transportLabel(proposal.transport)}
			</DetailRow>
			<DetailRow
				label="Description"
				className="sm:col-span-2"
				valueClassName="font-normal leading-[1.5]"
			>
				{proposal.description || "No description provided"}
			</DetailRow>
			{proposal.tool_allow_list && proposal.tool_allow_list.length > 0 && (
				<DetailRow label="Allowed tools" className="sm:col-span-2">
					<div className="flex flex-wrap gap-1">
						{proposal.tool_allow_list.map((tool) => (
							<Badge key={tool} size="sm" variant="green">
								{tool}
							</Badge>
						))}
					</div>
				</DetailRow>
			)}
			{proposal.tool_deny_list && proposal.tool_deny_list.length > 0 && (
				<DetailRow label="Denied tools" className="sm:col-span-2">
					<div className="flex flex-wrap gap-1">
						{proposal.tool_deny_list.map((tool) => (
							<Badge key={tool} size="sm" variant="destructive">
								{tool}
							</Badge>
						))}
					</div>
				</DetailRow>
			)}
			{providedAuthMaterial.length > 0 && (
				<DetailRow label="Provided credentials" className="sm:col-span-2">
					<div className="flex flex-wrap gap-1">
						{providedAuthMaterial.map((material) => (
							<Badge key={material} size="sm">
								{material}
							</Badge>
						))}
					</div>
				</DetailRow>
			)}
		</section>
	);
};

const DetailRow: FC<{
	label: string;
	children: ReactNode;
	className?: string;
	valueClassName?: string;
}> = ({ label, children, className = "", valueClassName = "" }) => {
	return (
		<div className={`flex min-w-0 flex-col gap-1.5 ${className}`}>
			<span className="text-2xs font-semibold leading-none uppercase tracking-[0.06em] text-content-secondary">
				{label}
			</span>
			<div
				className={`break-words text-sm leading-[1.3] text-content-primary ${valueClassName}`}
			>
				{children}
			</div>
		</div>
	);
};

const ProposalSummary: FC<{ proposal: TypesGen.MCPServerProposal }> = ({
	proposal,
}) => {
	let host = proposal.url || "N/A";
	try {
		host = new URL(proposal.url).host || host;
	} catch {
		host = proposal.url || "N/A";
	}

	const authentication =
		AUTH_TYPE_LABELS[proposal.auth_type] ?? proposal.auth_type;
	const authenticationDescription =
		proposal.auth_type === "none"
			? "No credentials required"
			: "Stored by Coder, not the agent";

	return (
		<section aria-labelledby="proposal-summary-heading">
			<h2
				id="proposal-summary-heading"
				className="m-0 mb-[14px] text-sm font-semibold leading-none text-content-primary"
			>
				Summary
			</h2>
			<div className="flex flex-col">
				<SummaryRow
					icon={<GlobeIcon />}
					label="Host"
					value={host}
					valueClassName="break-all font-mono text-xs leading-[1.3]"
				/>
				<SummaryRow
					icon={<KeyRoundIcon />}
					label="Authentication"
					value={authentication}
					description={authenticationDescription}
				/>
				<SummaryRow
					icon={<UserRoundIcon />}
					label="Access"
					value="Only you"
					description="Not shared org-wide"
					last
				/>
			</div>
		</section>
	);
};

const SummaryRow: FC<{
	icon: ReactNode;
	label: string;
	value: string;
	description?: string;
	valueClassName?: string;
	last?: boolean;
}> = ({
	icon,
	label,
	value,
	description,
	valueClassName = "",
	last = false,
}) => (
	<div
		className={`flex gap-[11px] py-[11px] ${last ? "" : "border-0 border-b border-solid border-border-default"}`}
	>
		<span className="mt-px flex size-4 shrink-0 text-content-secondary [&>svg]:size-full">
			{icon}
		</span>
		<div className="min-w-0 flex-1">
			<div className="text-[11px] font-semibold leading-none uppercase tracking-[0.05em] text-content-secondary">
				{label}
			</div>
			<div
				className={`mt-1 text-[13px] font-semibold leading-[1.2] text-content-primary ${valueClassName}`}
			>
				{value}
			</div>
			{description && (
				<div className="mt-[3px] text-[11px] font-medium leading-[1.4] text-content-secondary">
					{description}
				</div>
			)}
		</div>
	</div>
);

export default MCPProposalPage;

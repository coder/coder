import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router";
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
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { Spinner } from "#/components/Spinner/Spinner";
import { MCPServerIcon } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerIcon";
import {
	AUTH_TYPE_LABELS,
	TRANSPORT_OPTIONS,
} from "#/pages/AISettingsPage/MCPServersPage/components/mcpServerFormLogic";
import { pageTitle } from "#/utils/page";

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
		acceptMutation.mutate(undefined, {
			onSuccess: (res) => {
				if (res.connect_url && !res.authenticated) {
					setRedirecting(true);
					redirectToConnectUrl(res.connect_url);
				}
			},
		});
	};

	return (
		<Margins size="medium" className="py-10">
			<title>{pageTitle("MCP Server Proposal")}</title>
			<MCPProposalPageContent
				proposalQuery={proposalQuery}
				acceptMutation={acceptMutation}
				rejectMutation={rejectMutation}
				redirecting={redirecting}
				onAccept={handleAccept}
				onReject={() => rejectMutation.mutate()}
			/>
		</Margins>
	);
};

interface MCPProposalPageContentProps {
	proposalQuery: ReturnType<typeof useQuery<TypesGen.MCPServerProposal>>;
	acceptMutation: ReturnType<
		typeof useMutation<TypesGen.AcceptMCPServerProposalResponse, unknown, void>
	>;
	rejectMutation: ReturnType<typeof useMutation<void, unknown, void>>;
	redirecting: boolean;
	onAccept: () => void;
	onReject: () => void;
}

const MCPProposalPageContent: FC<MCPProposalPageContentProps> = ({
	proposalQuery,
	acceptMutation,
	rejectMutation,
	redirecting,
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

	return (
		<div className="flex flex-col gap-6">
			<header className="flex items-center gap-4">
				<MCPServerIcon
					iconUrl={proposal.icon_url ?? ""}
					name={proposal.display_name}
					className="size-12"
				/>
				<div className="flex flex-col">
					<h1 className="m-0 text-2xl font-semibold text-content-primary">
						{proposal.display_name}
					</h1>
					<span className="text-sm text-content-secondary">
						MCP server proposal
					</span>
				</div>
			</header>

			<Alert severity="info">
				This MCP server will only be available to you.
			</Alert>

			{proposal.create_disabled && (
				<Alert severity="warning">
					This MCP server will be created in a disabled state.
				</Alert>
			)}

			<ProposalDetails proposal={proposal} />

			{acceptMutation.error !== null && (
				<ErrorAlert error={acceptMutation.error} />
			)}
			{rejectMutation.error !== null && (
				<ErrorAlert error={rejectMutation.error} />
			)}

			<div className="flex items-center gap-3">
				<Button
					onClick={onAccept}
					disabled={acceptMutation.isPending || rejectMutation.isPending}
				>
					<Spinner loading={acceptMutation.isPending} />
					Accept
				</Button>
				<Button
					variant="outline"
					onClick={onReject}
					disabled={acceptMutation.isPending || rejectMutation.isPending}
				>
					<Spinner loading={rejectMutation.isPending} />
					Reject
				</Button>
			</div>
		</div>
	);
};

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
			className="flex flex-col gap-5 rounded-lg border border-solid border-border-default bg-surface-secondary p-6"
		>
			<DetailRow label="Display name">{proposal.display_name}</DetailRow>
			<DetailRow label="Slug">{proposal.slug}</DetailRow>
			<DetailRow label="URL">{proposal.url}</DetailRow>
			<DetailRow label="Description">
				{proposal.description || "No description provided"}
			</DetailRow>
			<DetailRow label="Transport">
				<Badge size="sm">{transportLabel(proposal.transport)}</Badge>
			</DetailRow>
			<DetailRow label="Authentication">
				<Badge size="sm">
					{AUTH_TYPE_LABELS[proposal.auth_type] ?? proposal.auth_type}
				</Badge>
			</DetailRow>
			{proposal.tool_allow_list && proposal.tool_allow_list.length > 0 && (
				<DetailRow label="Allowed tools">
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
				<DetailRow label="Denied tools">
					<div className="flex flex-wrap gap-1">
						{proposal.tool_deny_list.map((tool) => (
							<Badge key={tool} size="sm" variant="destructive">
								{tool}
							</Badge>
						))}
					</div>
				</DetailRow>
			)}
			<DetailRow label="Provided credentials">
				{providedAuthMaterial.length > 0 ? (
					<div className="flex flex-wrap gap-1">
						{providedAuthMaterial.map((material) => (
							<Badge key={material} size="sm">
								{material}
							</Badge>
						))}
					</div>
				) : (
					"None"
				)}
			</DetailRow>
		</section>
	);
};

const DetailRow: FC<{ label: string; children: ReactNode }> = ({
	label,
	children,
}) => {
	return (
		<div className="flex flex-col gap-1">
			<span className="text-xs font-medium uppercase text-content-secondary">
				{label}
			</span>
			<div className="break-all text-sm text-content-primary">{children}</div>
		</div>
	);
};

export default MCPProposalPage;

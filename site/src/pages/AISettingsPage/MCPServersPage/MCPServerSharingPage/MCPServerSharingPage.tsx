import { type FC, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	mcpServerConfig,
	mcpServerConfigACL,
	updateMCPServerConfigACL,
} from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ResourceSharingPageView } from "#/pages/AISettingsPage/components/ResourceSharingPage/ResourceSharingPageView";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";

const MCPServerSharingPage: FC = () => {
	const { serverId } = useParams<{ serverId: string }>();
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
	const serverQuery = useQuery({
		...mcpServerConfig(serverId ?? ""),
		enabled: Boolean(serverId),
	});
	const server = serverQuery.data;
	const organization = organizations.find(
		(candidate) => candidate.id === server?.organization_id,
	);

	useEffect(() => {
		if (
			!organization ||
			searchParams.get("organization") === organization.name
		) {
			return;
		}
		setSearchParams(
			(current) => {
				const next = new URLSearchParams(current);
				next.set("organization", organization.name);
				return next;
			},
			{ replace: true },
		);
	}, [organization, searchParams, setSearchParams]);

	const permissionQuery = useQuery({
		...checkAuthorization({
			checks: {
				canEdit: {
					object: {
						resource_type: "mcp_server_config",
						resource_id: server?.id,
						organization_id: server?.organization_id,
					},
					action: "update",
				},
				canShare: {
					object: {
						resource_type: "mcp_server_config",
						resource_id: server?.id,
						organization_id: server?.organization_id,
					},
					action: "share",
				},
			},
		}),
		enabled: Boolean(server && isTemplateRBACEnabled),
	});
	const aclQuery = useQuery({
		...mcpServerConfigACL(serverId ?? ""),
		enabled: Boolean(serverId && server && isTemplateRBACEnabled),
	});
	const updateMutation = useMutation(updateMCPServerConfigACL(queryClient));

	if (!serverId) {
		return <ErrorAlert error={new Error("MCP server ID is required.")} />;
	}
	if (serverQuery.isLoading) {
		return <Loader />;
	}
	if (serverQuery.error) {
		return <ErrorAlert error={serverQuery.error} />;
	}
	if (!server) {
		return <ErrorAlert error={new Error("MCP server not found.")} />;
	}
	if (!organization) {
		return (
			<ErrorAlert error={new Error("MCP server organization not found.")} />
		);
	}

	return (
		<>
			<title>{pageTitle(`Share ${server.display_name}`, "AI Settings")}</title>
			{!isTemplateRBACEnabled ? (
				<PaywallPremium
					message="MCP server sharing"
					description="Control user and group access to MCP servers. You need a Premium license to use this feature."
					documentationLink={docs("/admin/templates/template-permissions")}
				/>
			) : (
				<ResourceSharingPageView
					resourceName={server.display_name}
					resourceTypeLabel="MCP server"
					backPath={
						permissionQuery.data?.canEdit
							? `/ai/settings/mcp-servers/${server.id}`
							: "/ai/settings/mcp-servers"
					}
					search={location.search}
					organizationId={server.organization_id}
					acl={aclQuery.data}
					isLoading={aclQuery.isLoading || permissionQuery.isLoading}
					error={aclQuery.error ?? permissionQuery.error}
					mutationError={updateMutation.error}
					canShare={Boolean(permissionQuery.data?.canShare)}
					isMutating={updateMutation.isPending}
					onAddUser={async (userId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							serverId: server.id,
							req: { user_roles: { [userId]: "read" } },
						});
						toast.success("User granted MCP server access.");
					}}
					onAddGroup={async (groupId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							serverId: server.id,
							req: { group_roles: { [groupId]: "read" } },
						});
						toast.success("Group granted MCP server access.");
					}}
					onRemoveUser={async (userId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							serverId: server.id,
							req: { user_roles: { [userId]: "" } },
						});
						toast.success("User MCP server access removed.");
					}}
					onRemoveGroup={async (groupId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							serverId: server.id,
							req: { group_roles: { [groupId]: "" } },
						});
						toast.success("Group MCP server access removed.");
					}}
				/>
			)}
		</>
	);
};

export default MCPServerSharingPage;

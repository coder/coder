import { type FC, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	chatModelConfig,
	chatModelConfigACL,
	updateChatModelConfigACL,
} from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ResourceSharingPageView } from "#/pages/AISettingsPage/components/ResourceSharingPage/ResourceSharingPageView";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";

const ModelSharingPage: FC = () => {
	const { modelId } = useParams<{ modelId: string }>();
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
	const modelQuery = useQuery({
		...chatModelConfig(modelId ?? ""),
		enabled: Boolean(modelId),
	});
	const model = modelQuery.data;
	const organization = organizations.find(
		(candidate) => candidate.id === model?.organization_id,
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
						resource_type: "chat_model_config",
						resource_id: model?.id,
						organization_id: model?.organization_id,
					},
					action: "update",
				},
				canShare: {
					object: {
						resource_type: "chat_model_config",
						resource_id: model?.id,
						organization_id: model?.organization_id,
					},
					action: "share",
				},
			},
		}),
		enabled: Boolean(model && isTemplateRBACEnabled),
	});
	const aclQuery = useQuery({
		...chatModelConfigACL(modelId ?? ""),
		enabled: Boolean(modelId && model && isTemplateRBACEnabled),
	});
	const updateMutation = useMutation(updateChatModelConfigACL(queryClient));

	if (!modelId) {
		return <ErrorAlert error={new Error("Model ID is required.")} />;
	}
	if (modelQuery.isLoading) {
		return <Loader />;
	}
	if (modelQuery.error) {
		return <ErrorAlert error={modelQuery.error} />;
	}
	if (!model) {
		return <ErrorAlert error={new Error("Model not found.")} />;
	}
	if (!organization) {
		return <ErrorAlert error={new Error("Model organization not found.")} />;
	}

	const name = model.display_name || model.model;
	return (
		<>
			<title>{pageTitle(`Share ${name}`, "AI Settings")}</title>
			{!isTemplateRBACEnabled ? (
				<PaywallPremium
					message="Model sharing"
					description="Control user and group access to models. You need a Premium license to use this feature."
					documentationLink={docs("/admin/templates/template-permissions")}
				/>
			) : (
				<ResourceSharingPageView
					resourceName={name}
					resourceTypeLabel="model"
					backPath={
						permissionQuery.data?.canEdit
							? `/ai/settings/models/${model.id}`
							: "/ai/settings/models"
					}
					search={location.search}
					organizationId={model.organization_id}
					acl={aclQuery.data}
					isLoading={aclQuery.isLoading || permissionQuery.isLoading}
					error={aclQuery.error ?? permissionQuery.error}
					mutationError={updateMutation.error}
					canShare={Boolean(permissionQuery.data?.canShare)}
					isMutating={updateMutation.isPending}
					onAddUser={async (userId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							modelConfigId: model.id,
							req: { user_roles: { [userId]: "read" } },
						});
						toast.success("User granted model access.");
					}}
					onAddGroup={async (groupId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							modelConfigId: model.id,
							req: { group_roles: { [groupId]: "read" } },
						});
						toast.success("Group granted model access.");
					}}
					onRemoveUser={async (userId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							modelConfigId: model.id,
							req: { user_roles: { [userId]: "" } },
						});
						toast.success("User model access removed.");
					}}
					onRemoveGroup={async (groupId) => {
						await updateMutation.mutateAsync({
							organization: organization.name,
							modelConfigId: model.id,
							req: { group_roles: { [groupId]: "" } },
						});
						toast.success("Group model access removed.");
					}}
				/>
			)}
		</>
	);
};

export default ModelSharingPage;

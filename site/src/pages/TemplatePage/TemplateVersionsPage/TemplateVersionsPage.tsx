import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	templateVersions,
	templateVersionsQueryKey,
} from "#/api/queries/templates";
import type { TemplateVersion } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { getTemplatePageTitle } from "../utils";
import { VersionsTable } from "./VersionsTable";

const TemplateVersionsPage = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { template, permissions } = useTemplateLayoutContext();
	const queryClient = useQueryClient();
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);
	const { data } = useQuery(templateVersions(template.id));
	// We use this to update the active version in the UI without having to refetch the template
	const [latestActiveVersion, setLatestActiveVersion] = useState(
		template.active_version_id,
	);
	const [versionToPromote, setVersionToPromote] = useState<
		TemplateVersion | undefined
	>();
	const [versionToArchive, setVersionToArchive] = useState<
		TemplateVersion | undefined
	>();

	const { mutateAsync: promoteVersion, isPending: isPromoting } = useMutation({
		mutationFn: (templateVersionId: string) => {
			return API.updateActiveTemplateVersion(template.id, {
				id: templateVersionId,
			});
		},
	});

	const { mutateAsync: archiveVersion, isPending: isArchiving } = useMutation({
		mutationFn: (templateVersionId: string) => {
			return API.archiveTemplateVersion(templateVersionId);
		},
	});

	return (
		<>
			<title>{getTemplatePageTitle("Versions", template)}</title>

			<VersionsTable
				versions={data}
				onPromoteClick={
					permissions.canUpdateTemplate ? setVersionToPromote : undefined
				}
				onArchiveClick={
					permissions.canUpdateTemplate ? setVersionToArchive : undefined
				}
				activeVersionId={latestActiveVersion}
			/>
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={Boolean(versionToPromote)}
				onConfirm={async () => {
					if (!versionToPromote) {
						return;
					}
					const { id, name } = versionToPromote;
					try {
						await promoteVersion(id);
						setLatestActiveVersion(id);
						setVersionToPromote(undefined);
						toast.success(`Version "${name}" promoted successfully.`, {
							action: {
								label: "View template",
								onClick: () => navigate(templateLink),
							},
						});
					} catch (error) {
						toast.error(
							getErrorMessage(error, `Failed to promote version "${name}".`),
							{
								description: getErrorDetail(error),
							},
						);
					}
				}}
				onClose={() => setVersionToPromote(undefined)}
				title="Promote version"
				confirmLoading={isPromoting}
				confirmText="Promote"
				description={
					<>
						Are you sure you want to promote version{" "}
						<strong>{versionToPromote?.name}</strong>? Workspaces will be
						prompted to “Update” to this version once promoted.
					</>
				}
			/>
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={Boolean(versionToArchive)}
				onConfirm={async () => {
					if (!versionToArchive) {
						return;
					}
					const { id, name } = versionToArchive;
					try {
						await archiveVersion(id);
						await queryClient.invalidateQueries({
							queryKey: templateVersionsQueryKey(template.id),
						});
						setVersionToArchive(undefined);
						toast.success(`Version "${name}" archived successfully.`);
					} catch (error) {
						toast.error(
							getErrorMessage(error, `Failed to archive version "${name}".`),
							{
								description: getErrorDetail(error),
							},
						);
					}
				}}
				onClose={() => setVersionToArchive(undefined)}
				title="Archive version"
				confirmLoading={isArchiving}
				confirmText="Archive"
				description={
					<>
						Are you sure you want to archive version{" "}
						<strong>{versionToArchive?.name}</strong>? This is reversible.
						Archived versions cannot be used by workspaces.
					</>
				}
			/>
		</>
	);
};

export default TemplateVersionsPage;

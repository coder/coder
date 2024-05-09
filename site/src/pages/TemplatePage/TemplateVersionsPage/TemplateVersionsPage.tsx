import { useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery } from "react-query";
import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { getTemplatePageTitle } from "../utils";
import { VersionsTable } from "./VersionsTable";

const TemplateVersionsPage = () => {
  const { template, permissions } = useTemplateLayoutContext();
  const { data } = useQuery({
    queryKey: ["template", "versions", template.id],
    queryFn: () => API.getTemplateVersions(template.id),
  });
  // We use this to update the active version in the UI without having to refetch the template
  const [latestActiveVersion, setLatestActiveVersion] = useState(
    template.active_version_id,
  );
  const { mutate: promoteVersion, isLoading: isPromoting } = useMutation({
    mutationFn: (templateVersionId: string) => {
      return API.updateActiveTemplateVersion(template.id, {
        id: templateVersionId,
      });
    },
    onSuccess: async () => {
      setLatestActiveVersion(selectedVersionIdToPromote as string);
      setSelectedVersionIdToPromote(undefined);
      displaySuccess("Version promoted successfully");
    },
    onError: (error) => {
      displayError(getErrorMessage(error, "Failed to promote version"));
    },
  });

  const { mutate: archiveVersion, isLoading: isArchiving } = useMutation({
    mutationFn: (templateVersionId: string) => {
      return API.archiveTemplateVersion(templateVersionId);
    },
    onSuccess: async () => {
      // The reload is unfortunate. When a version is archived, we should hide
      // the row. I do not know an easy way to do that, so a reload makes the API call
      // resend and now the version is omitted.
      // TODO: Improve this to not reload the page.
      location.reload();
      setSelectedVersionIdToArchive(undefined);
      displaySuccess("Version archived successfully");
    },
    onError: (error) => {
      displayError(getErrorMessage(error, "Failed to archive version"));
    },
  });

  const [selectedVersionIdToPromote, setSelectedVersionIdToPromote] = useState<
    string | undefined
  >();
  const [selectedVersionIdToArchive, setSelectedVersionIdToArchive] = useState<
    string | undefined
  >();

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Versions", template)}</title>
      </Helmet>
      <VersionsTable
        versions={data}
        onPromoteClick={
          permissions.canUpdateTemplate
            ? setSelectedVersionIdToPromote
            : undefined
        }
        onArchiveClick={
          permissions.canUpdateTemplate
            ? setSelectedVersionIdToArchive
            : undefined
        }
        activeVersionId={latestActiveVersion}
      />
      {/* Promote confirm */}
      <ConfirmDialog
        type="info"
        hideCancel={false}
        open={selectedVersionIdToPromote !== undefined}
        onConfirm={() => {
          promoteVersion(selectedVersionIdToPromote as string);
        }}
        onClose={() => setSelectedVersionIdToPromote(undefined)}
        title="Promote version"
        confirmLoading={isPromoting}
        confirmText="Promote"
        description="Are you sure you want to promote this version? Workspaces will be prompted to “Update” to this version once promoted."
      />
      {/* Archive Confirm */}
      <ConfirmDialog
        type="info"
        hideCancel={false}
        open={selectedVersionIdToArchive !== undefined}
        onConfirm={() => {
          archiveVersion(selectedVersionIdToArchive as string);
        }}
        onClose={() => setSelectedVersionIdToArchive(undefined)}
        title="Archive version"
        confirmLoading={isArchiving}
        confirmText="Archive"
        description="Are you sure you want to archive this version (this is reversible)? Archived versions cannot be used by workspaces."
      />
    </>
  );
};

export default TemplateVersionsPage;

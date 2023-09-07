import { useMutation, useQuery } from "@tanstack/react-query";
import { getTemplateVersions, updateActiveTemplateVersion } from "api/api";
import { getErrorMessage } from "api/errors";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout";
import { VersionsTable } from "./VersionsTable";
import { useState } from "react";
import { Helmet } from "react-helmet-async";
import { getTemplatePageTitle } from "../utils";

const TemplateVersionsPage = () => {
  const { template, permissions } = useTemplateLayoutContext();
  const { data } = useQuery({
    queryKey: ["template", "versions", template.id],
    queryFn: () => getTemplateVersions(template.id),
  });
  // We use this to update the active version in the UI without having to refetch the template
  const [latestActiveVersion, setLatestActiveVersion] = useState(
    template.active_version_id,
  );
  const { mutate: promoteVersion, isLoading: isPromoting } = useMutation({
    mutationFn: (templateVersionId: string) => {
      return updateActiveTemplateVersion(template.id, {
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
  const [selectedVersionIdToPromote, setSelectedVersionIdToPromote] = useState<
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
        activeVersionId={latestActiveVersion}
      />
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
    </>
  );
};

export default TemplateVersionsPage;

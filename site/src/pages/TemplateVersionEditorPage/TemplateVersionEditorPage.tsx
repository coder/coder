import { useMachine } from "@xstate/react";
import { TemplateVersionEditor } from "./TemplateVersionEditor";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { templateVersionEditorMachine } from "xServices/templateVersionEditor/templateVersionEditorXService";
import { useQuery } from "react-query";
import { templateByName, templateVersionByName } from "api/queries/templates";
import { file } from "api/queries/files";
import { TarReader } from "utils/tar";
import { FileTree } from "utils/filetree";
import { createTemplateVersionFileTree } from "utils/templateVersion";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionEditorPage: FC = () => {
  const navigate = useNavigate();
  const { version: versionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();
  const templateQuery = useQuery(templateByName(orgId, templateName));
  const templateVersionQuery = useQuery(
    templateVersionByName(orgId, templateName, versionName),
  );
  const fileQuery = useQuery({
    ...file(templateVersionQuery.data?.job.file_id ?? ""),
    enabled: templateVersionQuery.isSuccess,
  });
  const [editorState, sendEvent] = useMachine(templateVersionEditorMachine, {
    context: { orgId },
  });
  const [fileTree, setFileTree] = useState<FileTree>();

  useEffect(() => {
    const initialize = async (file: ArrayBuffer) => {
      const tarReader = new TarReader();
      await tarReader.readFile(file);
      const fileTree = await createTemplateVersionFileTree(tarReader);
      sendEvent({ type: "INITIALIZE", tarReader });
      setFileTree(fileTree);
    };

    if (fileQuery.data) {
      initialize(fileQuery.data).catch(() => {
        console.error("Error on initializing the editor");
      });
    }
  }, [fileQuery.data, sendEvent]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {templateQuery.data && templateVersionQuery.data && fileTree && (
        <TemplateVersionEditor
          template={templateQuery.data}
          templateVersion={
            editorState.context.version || templateVersionQuery.data
          }
          isBuildingNewVersion={Boolean(editorState.context.version)}
          defaultFileTree={fileTree}
          onPreview={(fileTree) => {
            sendEvent({
              type: "CREATE_VERSION",
              fileTree,
              templateId: templateQuery.data.id,
            });
          }}
          onPublish={() => {
            sendEvent({
              type: "PUBLISH",
            });
          }}
          onCancelPublish={() => {
            sendEvent({
              type: "CANCEL_PUBLISH",
            });
          }}
          onConfirmPublish={(data) => {
            sendEvent({
              type: "CONFIRM_PUBLISH",
              ...data,
            });
          }}
          isAskingPublishParameters={editorState.matches(
            "askPublishParameters",
          )}
          isPublishing={editorState.matches("publishingVersion")}
          publishingError={editorState.context.publishingError}
          publishedVersion={editorState.context.lastSuccessfulPublishedVersion}
          onCreateWorkspace={() => {
            const params = new URLSearchParams();
            const publishedVersion =
              editorState.context.lastSuccessfulPublishedVersion;
            if (publishedVersion) {
              params.set("version", publishedVersion.id);
            }
            navigate(
              `/templates/${templateName}/workspace?${params.toString()}`,
            );
          }}
          disablePreview={editorState.hasTag("loading")}
          disableUpdate={
            editorState.hasTag("loading") ||
            editorState.context.version?.job.status !== "succeeded"
          }
          resources={editorState.context.resources}
          buildLogs={editorState.context.buildLogs}
          isPromptingMissingVariables={editorState.matches("promptVariables")}
          missingVariables={editorState.context.missingVariables}
          onSubmitMissingVariableValues={(values) => {
            sendEvent({
              type: "SET_MISSING_VARIABLE_VALUES",
              values,
            });
          }}
          onCancelSubmitMissingVariableValues={() => {
            sendEvent({
              type: "CANCEL_MISSING_VARIABLE_VALUES",
            });
          }}
        />
      )}
    </>
  );
};

export default TemplateVersionEditorPage;

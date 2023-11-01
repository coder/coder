import { useMachine } from "@xstate/react";
import { TemplateVersionEditor } from "./TemplateVersionEditor";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC, useEffect, useRef, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { templateVersionEditorMachine } from "xServices/templateVersionEditor/templateVersionEditorXService";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  createTemplateVersion,
  resources,
  templateByName,
  templateVersionByName,
  templateVersionVariables,
} from "api/queries/templates";
import { file, uploadFile } from "api/queries/files";
import { TarReader, TarWriter } from "utils/tar";
import { FileTree, traverse } from "utils/filetree";
import {
  createTemplateVersionFileTree,
  isAllowedFile,
} from "utils/templateVersion";
import { watchBuildLogsByTemplateVersionId } from "api/api";
import { ProvisionerJobLog } from "api/typesGenerated";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionEditorPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { version: initialVersionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();
  const [currentVersionName, setCurrentVersionName] =
    useState(initialVersionName);
  const templateQuery = useQuery(templateByName(orgId, templateName));
  const templateVersionOptions = templateVersionByName(
    orgId,
    templateName,
    currentVersionName,
  );
  const templateVersionQuery = useQuery({
    ...templateVersionOptions,
    keepPreviousData: true,
  });
  const fileQuery = useQuery({
    ...file(templateVersionQuery.data?.job.file_id ?? ""),
    enabled: templateVersionQuery.isSuccess,
  });
  const [editorState, sendEvent] = useMachine(templateVersionEditorMachine, {
    context: { orgId, templateId: templateQuery.data?.id },
  });
  const [fileTree, setFileTree] = useState<FileTree>();
  const uploadFileMutation = useMutation(uploadFile());
  const currentTarFileRef = useRef<TarReader | null>(null);
  const createTemplateVersionMutation = useMutation(
    createTemplateVersion(orgId),
  );
  const resourcesQuery = useQuery({
    ...resources(templateVersionQuery.data?.id ?? ""),
    enabled: templateVersionQuery.data?.job.status === "succeeded",
  });

  // Initialize file tree
  useEffect(() => {
    const initializeFileTree = async (file: ArrayBuffer) => {
      const tarReader = new TarReader();
      await tarReader.readFile(file);
      currentTarFileRef.current = tarReader;
      const fileTree = await createTemplateVersionFileTree(tarReader);
      setFileTree(fileTree);
    };

    if (fileQuery.data) {
      initializeFileTree(fileQuery.data).catch(() => {
        console.error("Error on initializing the editor");
      });
    }
  }, [fileQuery.data, sendEvent]);

  // Watch version logs
  const [logs, setLogs] = useState<ProvisionerJobLog[]>([]);
  const templateVersionId = templateVersionQuery.data?.id;
  const refetchTemplateVersion = templateVersionQuery.refetch;
  const templateVersionStatus = templateVersionQuery.data?.job.status;
  useEffect(() => {
    if (!templateVersionId || !templateVersionStatus) {
      return;
    }

    if (templateVersionStatus !== "running") {
      return;
    }

    setLogs([]);
    const socket = watchBuildLogsByTemplateVersionId(templateVersionId, {
      onMessage: (log) => {
        setLogs((logs) => [...logs, log]);
      },
      onDone: async () => {
        await refetchTemplateVersion();
      },
      onError: (error) => {
        console.error(error);
      },
    });

    return () => {
      socket.close();
    };
  }, [refetchTemplateVersion, templateVersionId, templateVersionStatus]);

  // Handling missing variables
  const missingVariablesQuery = useQuery({
    ...templateVersionVariables(templateVersionQuery.data?.id ?? ""),
    enabled:
      templateVersionQuery.data?.job.error_code ===
      "REQUIRED_TEMPLATE_VARIABLES",
  });
  const [isMissingVariablesDialogOpen, setIsMissingVariablesDialogOpen] =
    useState(false);
  useEffect(() => {
    if (missingVariablesQuery.data) {
      setIsMissingVariablesDialogOpen(true);
    }
  }, [missingVariablesQuery.data]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {templateQuery.data && templateVersionQuery.data && fileTree && (
        <TemplateVersionEditor
          template={templateQuery.data}
          templateVersion={templateVersionQuery.data}
          isBuildingNewVersion={
            templateVersionQuery.data.job.status === "running"
          }
          defaultFileTree={fileTree}
          onPreview={async (newFileTree) => {
            if (!currentTarFileRef.current) {
              return;
            }
            const newVersionFile = await generateVersionFiles(
              currentTarFileRef.current,
              newFileTree,
            );
            const serverFile = await uploadFileMutation.mutateAsync(
              newVersionFile,
            );
            const newVersion = await createTemplateVersionMutation.mutateAsync({
              provisioner: "terraform",
              storage_method: "file",
              tags: {},
              template_id: templateQuery.data.id,
              file_id: serverFile.hash,
            });

            setCurrentVersionName(newVersion.name);
            queryClient.setQueryData(
              templateVersionOptions.queryKey,
              newVersion,
            );
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
            templateVersionQuery.data.job.status === "running" ||
            templateVersionQuery.data.job.status !== "succeeded"
          }
          resources={resourcesQuery.data}
          buildLogs={logs}
          isPromptingMissingVariables={isMissingVariablesDialogOpen}
          missingVariables={missingVariablesQuery.data}
          onSubmitMissingVariableValues={async (values) => {
            if (!uploadFileMutation.data) {
              return;
            }

            const newVersion = await createTemplateVersionMutation.mutateAsync({
              provisioner: "terraform",
              storage_method: "file",
              tags: {},
              template_id: templateQuery.data.id,
              file_id: uploadFileMutation.data.hash,
              user_variable_values: values,
            });
            setCurrentVersionName(newVersion.name);
            queryClient.setQueryData(
              templateVersionOptions.queryKey,
              newVersion,
            );
            setIsMissingVariablesDialogOpen(false);
          }}
          onCancelSubmitMissingVariableValues={() => {
            setIsMissingVariablesDialogOpen(false);
          }}
        />
      )}
    </>
  );
};

const generateVersionFiles = async (
  tarReader: TarReader,
  fileTree: FileTree,
) => {
  const tar = new TarWriter();

  // Add previous non editable files
  for (const file of tarReader.fileInfo) {
    if (!isAllowedFile(file.name)) {
      if (file.type === "5") {
        tar.addFolder(file.name, {
          mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
          mtime: file.mtime,
          user: file.user,
          group: file.group,
        });
      } else {
        tar.addFile(file.name, tarReader.getTextFile(file.name) as string, {
          mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
          mtime: file.mtime,
          user: file.user,
          group: file.group,
        });
      }
    }
  }
  // Add the editable files
  traverse(fileTree, (content, _filename, fullPath) => {
    // When a file is deleted. Don't add it to the tar.
    if (content === undefined) {
      return;
    }

    if (typeof content === "string") {
      tar.addFile(fullPath, content);
      return;
    }

    tar.addFolder(fullPath);
  });
  const blob = (await tar.write()) as Blob;
  return new File([blob], "template.tar");
};

export default TemplateVersionEditorPage;

import { TemplateVersionEditor } from "./TemplateVersionEditor";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC, useEffect, useRef, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
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
import {
  patchTemplateVersion,
  updateActiveTemplateVersion,
  watchBuildLogsByTemplateVersionId,
} from "api/api";
import {
  PatchTemplateVersionRequest,
  ProvisionerJobLog,
  TemplateVersion,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";

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
  const uploadFileMutation = useMutation(uploadFile());
  const createTemplateVersionMutation = useMutation(
    createTemplateVersion(orgId),
  );
  const resourcesQuery = useQuery({
    ...resources(templateVersionQuery.data?.id ?? ""),
    enabled: templateVersionQuery.data?.job.status === "succeeded",
  });
  const { logs, setLogs } = useVersionLogs(templateVersionQuery.data, {
    onDone: templateVersionQuery.refetch,
  });
  const { fileTree, tarFile } = useFileTree(templateVersionQuery.data);
  const {
    missingVariables,
    setIsMissingVariablesDialogOpen,
    isMissingVariablesDialogOpen,
  } = useMissingVariables(templateVersionQuery.data);

  // Handle template publishing
  const [isPublishingDialogOpen, setIsPublishingDialogOpen] = useState(false);
  const publishVersionMutation = useMutation({
    mutationFn: publishVersion,
  });
  const [lastSuccessfulPublishedVersion, setLastSuccessfulPublishedVersion] =
    useState<TemplateVersion>();

  // Optimistically update the template version data job status to make the
  // build action feels faster
  const onBuildStart = () => {
    setLogs([]);

    queryClient.setQueryData(templateVersionOptions.queryKey, () => {
      return {
        ...templateVersionQuery.data,
        job: {
          ...templateVersionQuery.data?.job,
          status: "pending",
        },
      };
    });
  };

  const onBuildEnds = (newVersion: TemplateVersion) => {
    setCurrentVersionName(newVersion.name);
    queryClient.setQueryData(templateVersionOptions.queryKey, newVersion);
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {templateQuery.data && templateVersionQuery.data && fileTree && (
        <TemplateVersionEditor
          template={templateQuery.data}
          templateVersion={templateVersionQuery.data}
          defaultFileTree={fileTree}
          onPreview={async (newFileTree) => {
            if (!tarFile) {
              return;
            }
            onBuildStart();
            const newVersionFile = await generateVersionFiles(
              tarFile,
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
            onBuildEnds(newVersion);
          }}
          onPublish={() => {
            setIsPublishingDialogOpen(true);
          }}
          onCancelPublish={() => {
            setIsPublishingDialogOpen(false);
          }}
          onConfirmPublish={async ({ isActiveVersion, ...data }) => {
            await publishVersionMutation.mutateAsync({
              isActiveVersion,
              data,
              version: templateVersionQuery.data,
            });
            setIsPublishingDialogOpen(false);
            setLastSuccessfulPublishedVersion(templateVersionQuery.data);
          }}
          isAskingPublishParameters={isPublishingDialogOpen}
          isPublishing={publishVersionMutation.isLoading}
          publishingError={publishVersionMutation.error}
          publishedVersion={lastSuccessfulPublishedVersion}
          onCreateWorkspace={() => {
            const params = new URLSearchParams();
            const publishedVersion = lastSuccessfulPublishedVersion;
            if (publishedVersion) {
              params.set("version", publishedVersion.id);
            }
            navigate(
              `/templates/${templateName}/workspace?${params.toString()}`,
            );
          }}
          disablePreview={
            templateVersionQuery.data.job.status === "running" ||
            templateVersionQuery.data.job.status === "pending" ||
            createTemplateVersionMutation.isLoading ||
            uploadFileMutation.isLoading
          }
          disableUpdate={
            templateVersionQuery.data.job.status !== "succeeded" ||
            templateVersionQuery.data.name === initialVersionName ||
            templateVersionQuery.data.name ===
              lastSuccessfulPublishedVersion?.name
          }
          resources={resourcesQuery.data}
          buildLogs={logs}
          isPromptingMissingVariables={isMissingVariablesDialogOpen}
          missingVariables={missingVariables}
          onSubmitMissingVariableValues={async (values) => {
            if (!uploadFileMutation.data) {
              return;
            }
            onBuildStart();
            const newVersion = await createTemplateVersionMutation.mutateAsync({
              provisioner: "terraform",
              storage_method: "file",
              tags: {},
              template_id: templateQuery.data.id,
              file_id: uploadFileMutation.data.hash,
              user_variable_values: values,
            });
            onBuildEnds(newVersion);
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

const useFileTree = (templateVersion: TemplateVersion | undefined) => {
  const tarFileRef = useRef<TarReader | null>(null);
  const fileQuery = useQuery({
    ...file(templateVersion?.job.file_id ?? ""),
    enabled: templateVersion !== undefined,
  });
  const [fileTree, setFileTree] = useState<FileTree>();
  useEffect(() => {
    const initializeFileTree = async (file: ArrayBuffer) => {
      const tarReader = new TarReader();
      await tarReader.readFile(file);
      tarFileRef.current = tarReader;
      const fileTree = await createTemplateVersionFileTree(tarReader);
      setFileTree(fileTree);
    };

    if (fileQuery.data) {
      initializeFileTree(fileQuery.data).catch(() => {
        displayError("Error on initializing the editor");
      });
    }
  }, [fileQuery.data]);

  return {
    fileTree,
    tarFile: tarFileRef.current,
  };
};

const useVersionLogs = (
  templateVersion: TemplateVersion | undefined,
  options: { onDone: () => Promise<unknown> },
) => {
  const [logs, setLogs] = useState<ProvisionerJobLog[]>();
  const templateVersionId = templateVersion?.id;
  const refetchTemplateVersion = options.onDone;
  const templateVersionStatus = templateVersion?.job.status;

  useEffect(() => {
    if (!templateVersionId || !templateVersionStatus) {
      return;
    }

    if (templateVersionStatus !== "running") {
      return;
    }

    const socket = watchBuildLogsByTemplateVersionId(templateVersionId, {
      onMessage: (log) => {
        setLogs((logs) => (logs ? [...logs, log] : [log]));
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

  return {
    logs,
    setLogs,
  };
};

const useMissingVariables = (templateVersion: TemplateVersion | undefined) => {
  const { data: missingVariables } = useQuery({
    ...templateVersionVariables(templateVersion?.id ?? ""),
    enabled: templateVersion?.job.error_code === "REQUIRED_TEMPLATE_VARIABLES",
  });
  const [isMissingVariablesDialogOpen, setIsMissingVariablesDialogOpen] =
    useState(false);

  useEffect(() => {
    if (missingVariables) {
      setIsMissingVariablesDialogOpen(true);
    }
  }, [missingVariables]);

  return {
    missingVariables,
    isMissingVariablesDialogOpen,
    setIsMissingVariablesDialogOpen,
  };
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

const publishVersion = async (options: {
  version: TemplateVersion;
  data: PatchTemplateVersionRequest;
  isActiveVersion: boolean;
}) => {
  const { version, data, isActiveVersion } = options;
  const haveChanges =
    data.name !== version.name || data.message !== version.message;

  return Promise.all([
    haveChanges ? patchTemplateVersion(version.id, data) : Promise.resolve(),
    isActiveVersion
      ? updateActiveTemplateVersion(version.template_id!, {
          id: version.id,
        })
      : Promise.resolve(),
  ]);
};

export default TemplateVersionEditorPage;

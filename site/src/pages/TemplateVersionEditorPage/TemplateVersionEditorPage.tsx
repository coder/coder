import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { patchTemplateVersion, updateActiveTemplateVersion } from "api/api";
import { file, uploadFile } from "api/queries/files";
import {
  createTemplateVersion,
  resources,
  templateByName,
  templateByNameKey,
  templateVersionByName,
  templateVersionVariables,
} from "api/queries/templates";
import type {
  PatchTemplateVersionRequest,
  TemplateVersion,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useWatchVersionLogs } from "modules/templates/useWatchVersionLogs";
import { type FileTree, traverse } from "utils/filetree";
import { pageTitle } from "utils/page";
import { TarReader, TarWriter } from "utils/tar";
import { createTemplateVersionFileTree } from "utils/templateVersion";
import { TemplateVersionEditor } from "./TemplateVersionEditor";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionEditorPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { version: versionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();
  const templateQuery = useQuery(templateByName(orgId, templateName));
  const templateVersionOptions = templateVersionByName(
    orgId,
    templateName,
    versionName,
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
  const logs = useWatchVersionLogs(templateVersionQuery.data, {
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
    onSuccess: async () => {
      await queryClient.invalidateQueries(
        templateByNameKey(orgId, templateName),
      );
    },
  });
  const [lastSuccessfulPublishedVersion, setLastSuccessfulPublishedVersion] =
    useState<TemplateVersion>();

  // File navigation
  const [searchParams, setSearchParams] = useSearchParams();
  // It can be undefined when a selected file is deleted
  const activePath: string | undefined =
    searchParams.get("path") ?? findInitialFile(fileTree ?? {});
  const onActivePathChange = (path: string | undefined) => {
    if (path) {
      searchParams.set("path", path);
    } else {
      searchParams.delete("path");
    }
    setSearchParams(searchParams);
  };

  const navigateToVersion = (version: TemplateVersion) => {
    return navigate(
      `/templates/${templateName}/versions/${version.name}/edit`,
      { replace: true },
    );
  };

  const onBuildEnds = (newVersion: TemplateVersion) => {
    queryClient.setQueryData(templateVersionOptions.queryKey, newVersion);
    navigateToVersion(newVersion);
  };

  // Provisioner Tags
  const [provisionerTags, setProvisionerTags] = useState<
    Record<string, string>
  >({});
  useEffect(() => {
    if (templateVersionQuery.data?.job.tags) {
      setProvisionerTags(templateVersionQuery.data.job.tags);
    }
  }, [templateVersionQuery.data?.job.tags]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {!(templateQuery.data && templateVersionQuery.data && fileTree) ? (
        <Loader fullscreen />
      ) : (
        <TemplateVersionEditor
          activePath={activePath}
          onActivePathChange={onActivePathChange}
          template={templateQuery.data}
          templateVersion={templateVersionQuery.data}
          defaultFileTree={fileTree}
          onPreview={async (newFileTree) => {
            if (!tarFile) {
              return;
            }
            const newVersionFile = await generateVersionFiles(
              tarFile,
              newFileTree,
            );
            const serverFile =
              await uploadFileMutation.mutateAsync(newVersionFile);
            const newVersion = await createTemplateVersionMutation.mutateAsync({
              provisioner: "terraform",
              storage_method: "file",
              tags: provisionerTags,
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
            const publishedVersion = {
              ...templateVersionQuery.data,
              ...data,
            };
            setIsPublishingDialogOpen(false);
            setLastSuccessfulPublishedVersion(publishedVersion);
            queryClient.setQueryData(
              templateVersionOptions.queryKey,
              publishedVersion,
            );
            navigateToVersion(publishedVersion);
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
          isBuilding={
            createTemplateVersionMutation.isLoading ||
            uploadFileMutation.isLoading ||
            templateVersionQuery.data.job.status === "running" ||
            templateVersionQuery.data.job.status === "pending"
          }
          canPublish={
            templateVersionQuery.data.job.status === "succeeded" &&
            templateQuery.data.active_version_id !==
              templateVersionQuery.data.id
          }
          resources={resourcesQuery.data}
          buildLogs={logs}
          isPromptingMissingVariables={isMissingVariablesDialogOpen}
          missingVariables={missingVariables}
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
            onBuildEnds(newVersion);
            setIsMissingVariablesDialogOpen(false);
          }}
          onCancelSubmitMissingVariableValues={() => {
            setIsMissingVariablesDialogOpen(false);
          }}
          provisionerTags={provisionerTags}
          onUpdateProvisionerTags={(tags) => {
            setProvisionerTags(tags);
          }}
        />
      )}
    </>
  );
};

const useFileTree = (templateVersion: TemplateVersion | undefined) => {
  const fileQuery = useQuery({
    ...file(templateVersion?.job.file_id ?? ""),
    enabled: templateVersion !== undefined,
  });
  const [state, setState] = useState<{
    fileTree?: FileTree;
    tarFile?: TarReader;
  }>({
    fileTree: undefined,
    tarFile: undefined,
  });
  useEffect(() => {
    const initializeFileTree = async (file: ArrayBuffer) => {
      const tarFile = new TarReader();
      await tarFile.readFile(file);
      const fileTree = await createTemplateVersionFileTree(tarFile);
      setState({ fileTree, tarFile });
    };

    if (fileQuery.data) {
      initializeFileTree(fileQuery.data).catch((reason) => {
        console.error(reason);
        displayError("Error on initializing the editor");
      });
    }
  }, [fileQuery.data]);

  return state;
};

const useMissingVariables = (templateVersion: TemplateVersion | undefined) => {
  const isRequiringVariables =
    templateVersion?.job.error_code === "REQUIRED_TEMPLATE_VARIABLES";
  const { data: variables } = useQuery({
    ...templateVersionVariables(templateVersion?.id ?? ""),
    enabled: isRequiringVariables,
  });
  const [isMissingVariablesDialogOpen, setIsMissingVariablesDialogOpen] =
    useState(false);

  useEffect(() => {
    if (isRequiringVariables) {
      setIsMissingVariablesDialogOpen(true);
    }
  }, [isRequiringVariables]);

  return {
    missingVariables: isRequiringVariables ? variables : undefined,
    isMissingVariablesDialogOpen,
    setIsMissingVariablesDialogOpen,
  };
};

const generateVersionFiles = async (
  tarReader: TarReader,
  fileTree: FileTree,
) => {
  const tar = new TarWriter();

  traverse(fileTree, (content, _filename, fullPath) => {
    // When a file is deleted. Don't add it to the tar.
    if (content === undefined) {
      return;
    }

    const baseFileInfo = tarReader.fileInfo.find((i) => i.name === fullPath);

    if (typeof content === "string") {
      tar.addFile(fullPath, content, baseFileInfo);
      return;
    }

    tar.addFolder(fullPath, baseFileInfo);
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
  const publishActions: Promise<unknown>[] = [];

  if (haveChanges) {
    publishActions.push(patchTemplateVersion(version.id, data));
  }

  if (isActiveVersion) {
    publishActions.push(
      updateActiveTemplateVersion(version.template_id!, {
        id: version.id,
      }),
    );
  }

  return Promise.all(publishActions);
};

const findInitialFile = (fileTree: FileTree): string | undefined => {
  let initialFile: string | undefined;

  traverse(fileTree, (content, filename, path) => {
    if (filename.endsWith(".tf")) {
      initialFile = path;
    }
  });

  return initialFile;
};

export default TemplateVersionEditorPage;

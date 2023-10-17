import * as API from "api/api";
import { FileTree, createFile } from "./filetree";
import { TarReader } from "./tar";
import { TemplateVersion } from "api/typesGenerated";

export const getTemplateFilesWithDiff = async (
  templateName: string,
  version: TemplateVersion,
) => {
  const previousVersion = await API.getPreviousTemplateVersionByName(
    version.organization_id!,
    templateName,
    version.name,
  );
  const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] = [];
  loadFilesPromises.push(getTemplateVersionFiles(version.job.file_id));
  if (previousVersion) {
    loadFilesPromises.push(
      getTemplateVersionFiles(previousVersion.job.file_id),
    );
  }
  const [currentFiles, previousFiles] = await Promise.all(loadFilesPromises);
  return {
    currentFiles,
    previousFiles,
  };
};

// Content by filename
export type TemplateVersionFiles = Record<string, string>;

export const getTemplateVersionFiles = async (
  fileId: string,
): Promise<TemplateVersionFiles> => {
  const files: TemplateVersionFiles = {};
  const tarFile = await API.getFile(fileId);
  const tarReader = new TarReader();
  await tarReader.readFile(tarFile);
  for (const file of tarReader.fileInfo) {
    if (isAllowedFile(file.name)) {
      files[file.name] = tarReader.getTextFile(file.name) as string;
    }
  }
  return files;
};

export const allowedExtensions = [
  "tf",
  "md",
  "mkd",
  "Dockerfile",
  "protobuf",
  "sh",
  "tpl",
] as const;

export type AllowedExtension = (typeof allowedExtensions)[number];

export const isAllowedFile = (name: string) => {
  return allowedExtensions.some((ext) => name.endsWith(ext));
};

export const createTemplateVersionFileTree = async (
  tarReader: TarReader,
): Promise<FileTree> => {
  let fileTree: FileTree = {};
  for (const file of tarReader.fileInfo) {
    if (isAllowedFile(file.name)) {
      fileTree = createFile(
        file.name,
        fileTree,
        tarReader.getTextFile(file.name) as string,
      );
    }
  }
  return fileTree;
};

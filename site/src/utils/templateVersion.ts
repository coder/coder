import * as API from "api/api";
import { TemplateVersion } from "api/typesGenerated";
import { FileTree, createFile } from "./filetree";
import { TarReader } from "./tar";

/**
 * Content by filename
 */
export type TemplateVersionFiles = Record<string, string>;

export const getTemplateVersionFiles = async (
  version: TemplateVersion,
): Promise<TemplateVersionFiles> => {
  const files: TemplateVersionFiles = {};
  const tarFile = await API.getFile(version.job.file_id);
  const tarReader = new TarReader();
  await tarReader.readFile(tarFile);
  for (const file of tarReader.fileInfo) {
    if (isAllowedFile(file.name)) {
      files[file.name] = tarReader.getTextFile(file.name) as string;
    }
  }
  return files;
};

export const allowedExtensions = ["tf", "md", "Dockerfile", "protobuf"];

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

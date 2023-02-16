import * as API from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import { FileTree, setFile } from "./filetree"
import { TarReader } from "./tar"

/**
 * Content by filename
 */
export type TemplateVersionFiles = Record<string, string>

export const getTemplateVersionFiles = async (
  version: TemplateVersion,
): Promise<TemplateVersionFiles> => {
  const files: TemplateVersionFiles = {}
  const tarFile = await API.getFile(version.job.file_id)
  const tarReader = new TarReader()
  await tarReader.readFile(tarFile)
  for (const file of tarReader.fileInfo) {
    if (isAllowedFile(file.name)) {
      files[file.name] = tarReader.getTextFile(file.name) as string
    }
  }
  return files
}

const allowedExtensions = ["tf", "md", "Dockerfile"]

export const isAllowedFile = (name: string) => {
  return allowedExtensions.some((ext) => name.endsWith(ext))
}

export const createTemplateVersionFileTree = async (
  tarReader: TarReader,
): Promise<FileTree> => {
  let fileTree: FileTree = {}
  for (const file of tarReader.fileInfo) {
    if (isAllowedFile(file.name)) {
      fileTree = setFile(
        file.name,
        tarReader.getTextFile(file.name) as string,
        fileTree,
      )
    }
  }
  return fileTree
}

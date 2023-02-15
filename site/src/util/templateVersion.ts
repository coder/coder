import * as API from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import untar, { File as UntarFile } from "js-untar"
import { FileTree, setFile } from "./filetree"

/**
 * Content by filename
 */
export type TemplateVersionFiles = Record<string, string>

export const getTemplateVersionFiles = async (
  version: TemplateVersion,
  allowedExtensions: string[],
  allowedFiles: string[],
): Promise<TemplateVersionFiles> => {
  const files: TemplateVersionFiles = {}
  const tarFile = await API.getFile(version.job.file_id)
  const blobs: Record<string, Blob> = {}

  await untar(tarFile).then(undefined, undefined, async (file) => {
    const paths = file.name.split("/")
    const filename = paths[paths.length - 1]
    const [_, extension] = filename.split(".")

    if (
      allowedExtensions.includes(extension) ||
      allowedFiles.includes(filename)
    ) {
      blobs[filename] = file.blob
    }
  })

  await Promise.all(
    Object.entries(blobs).map(async ([filename, blob]) => {
      files[filename] = await blob.text()
    }),
  )

  return files
}

const allowedExtensions = ["tf", "md", "Dockerfile"]

export const isAllowedFile = (name: string) => {
  return allowedExtensions.some((ext) => name.endsWith(ext))
}

export const createTemplateVersionFileTree = async (
  untarFiles: UntarFile[],
): Promise<FileTree> => {
  let fileTree: FileTree = {}
  const blobs: Record<string, Blob> = {}

  for (const untarFile of untarFiles) {
    if (isAllowedFile(untarFile.name)) {
      blobs[untarFile.name] = untarFile.blob
    }
  }

  await Promise.all(
    Object.entries(blobs).map(async ([fullPath, blob]) => {
      const content = await blob.text()
      fileTree = setFile(fullPath, content, fileTree)
    }),
  )

  return fileTree
}

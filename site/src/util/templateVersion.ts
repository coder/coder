import * as API from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import untar from "js-untar"
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

export const createTemplateVersionFileTree = async (
  version: TemplateVersion,
): Promise<FileTree> => {
  let fileTree: FileTree = {}
  const tarFile = await API.getFile(version.job.file_id)
  const blobs: Record<string, Blob> = {}

  await untar(tarFile).then(undefined, undefined, async (file) => {
    if (allowedExtensions.some((ext) => file.name.endsWith(ext))) {
      blobs[file.name] = file.blob
    }
  })

  // We don't want to get the blob text during untar to not block the main thread.
  // Also, by doing it here, we can make all the loading in parallel.
  await Promise.all(
    Object.entries(blobs).map(async ([fullPath, blob]) => {
      const content = await blob.text()
      fileTree = setFile(fullPath, content, fileTree)
    }),
  )

  return fileTree
}

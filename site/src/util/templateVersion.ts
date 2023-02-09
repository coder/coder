import * as API from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import untar from "js-untar"
import set from "lodash/set"
import has from "lodash/has"
import omit from "lodash/omit"
import get from "lodash/get"

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

export type TemplateVersionFileTree = {
  [key: string]: TemplateVersionFileTree | string
}

export const getTemplateVersionFileTree = async (
  version: TemplateVersion,
  allowedExtensions: string[],
  allowedFiles: string[],
): Promise<TemplateVersionFileTree> => {
  let fileTree: TemplateVersionFileTree = {}
  const tarFile = await API.getFile(version.job.file_id)
  const blobs: Record<string, Blob> = {}

  await untar(tarFile).then(undefined, undefined, async (file) => {
    const fullPath = file.name
    const paths = fullPath.split("/")
    const filename = paths[paths.length - 1]
    const [_, extension] = filename.split(".")

    if (
      allowedExtensions.includes(extension) ||
      allowedFiles.includes(filename)
    ) {
      blobs[fullPath] = file.blob
    }
  })

  await Promise.all(
    Object.entries(blobs).map(async ([fullPath, blob]) => {
      const paths = fullPath.split("/")
      const content = await blob.text()
      fileTree = set(fileTree, paths, content)
    }),
  )

  return fileTree
}

export const setFile = (
  path: string,
  content: string,
  fileTree: TemplateVersionFileTree,
): TemplateVersionFileTree => {
  return set(fileTree, path.split("/"), content)
}

export const existsFile = (path: string, fileTree: TemplateVersionFileTree) => {
  return has(fileTree, path.split("/"))
}

export const removeFile = (path: string, fileTree: TemplateVersionFileTree) => {
  return omit(fileTree, path.split("/"))
}

export const getFileContent = (
  path: string,
  fileTree: TemplateVersionFileTree,
) => {
  return get(fileTree, path.split("/")) as string | TemplateVersionFileTree
}

export const isFolder = (path: string, fileTree: TemplateVersionFileTree) => {
  const content = getFileContent(path, fileTree)
  return typeof content === "object"
}

export const traverse = (
  fileTree: TemplateVersionFileTree,
  callback: (
    content: TemplateVersionFileTree | string,
    filename: string,
    fullPath: string,
  ) => void,
  parent?: string,
) => {
  Object.keys(fileTree).forEach((filename) => {
    const fullPath = parent ? `${parent}/${filename}` : filename
    const content = fileTree[filename]
    callback(content, filename, fullPath)
    if (typeof content === "object") {
      traverse(content, callback, fullPath)
    }
  })
}

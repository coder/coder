import set from "lodash/set"
import has from "lodash/has"
import unset from "lodash/unset"
import get from "lodash/get"

export type FileTree = {
  [key: string]: FileTree | string
}

export const setFile = (
  path: string,
  content: FileTree | string,
  fileTree: FileTree,
): FileTree => {
  return set(fileTree, path.split("/"), content)
}

export const existsFile = (path: string, fileTree: FileTree) => {
  return has(fileTree, path.split("/"))
}

export const removeFile = (path: string, fileTree: FileTree) => {
  const updatedFileTree = { ...fileTree }
  unset(fileTree, path.split("/"))
  return updatedFileTree
}

export const moveFile = (
  currentPath: string,
  newPath: string,
  fileTree: FileTree,
) => {
  const content = getFileContent(currentPath, fileTree)
  fileTree = removeFile(currentPath, fileTree)
  fileTree = setFile(newPath, content, fileTree)
  return fileTree
}

export const getFileContent = (path: string, fileTree: FileTree) => {
  return get(fileTree, path.split("/")) as string | FileTree
}

export const isFolder = (path: string, fileTree: FileTree) => {
  const content = getFileContent(path, fileTree)
  return typeof content === "object"
}

export const traverse = (
  fileTree: FileTree,
  callback: (
    content: FileTree | string,
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

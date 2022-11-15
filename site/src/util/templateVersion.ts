import { getFile } from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import untar from "js-untar"

/**
 * Content by filename
 */
export type TemplateVersionFiles = Record<string, string>

export const getTemplateVersionFiles = async (
  version: TemplateVersion,
): Promise<TemplateVersionFiles> => {
  const files: TemplateVersionFiles = {}
  const tarFile = await getFile(version.job.file_id)
  await untar(tarFile).then(undefined, undefined, async (file) => {
    const paths = file.name.split("/")
    const filename = paths[paths.length - 1]
    files[filename] = file.readAsString()
  })
  return files
}

export const filterTemplateFilesByExtension = (
  files: TemplateVersionFiles,
  extensions: string[],
): TemplateVersionFiles => {
  return Object.keys(files).reduce((filteredFiles, filename) => {
    const [_, extension] = filename.split(".")

    return extensions.includes(extension)
      ? { ...filteredFiles, [filename]: files[filename] }
      : filteredFiles
  }, {} as TemplateVersionFiles)
}

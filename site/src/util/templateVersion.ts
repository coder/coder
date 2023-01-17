import { getFile } from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import untar from "js-untar"

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
  const tarFile = await getFile(version.job.file_id)
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

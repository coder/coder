import { useQuery, UseQueryOptions } from "@tanstack/react-query"
import { getFile, getTemplateByName, getTemplateVersionByName } from "api/api"
import { createTemplateVersionFileTree } from "util/templateVersion"
import untar, { File as UntarFile } from "js-untar"

const getTemplateVersionData = async (
  orgId: string,
  templateName: string,
  versionName: string,
) => {
  const [template, version] = await Promise.all([
    getTemplateByName(orgId, templateName),
    getTemplateVersionByName(orgId, templateName, versionName),
  ])
  const tarFile = await getFile(version.job.file_id)
  let untarFiles: UntarFile[] = []
  await untar(tarFile).then((files) => {
    untarFiles = files
  })
  const fileTree = await createTemplateVersionFileTree(untarFiles)

  return {
    template,
    version,
    fileTree,
    untarFiles,
  }
}

type GetTemplateVersionResponse = Awaited<
  ReturnType<typeof getTemplateVersionData>
>

type UseTemplateVersionDataParams = {
  orgId: string
  templateName: string
  versionName: string
}

export const useTemplateVersionData = (
  { templateName, versionName, orgId }: UseTemplateVersionDataParams,
  options?: UseQueryOptions<GetTemplateVersionResponse>,
) => {
  return useQuery({
    queryKey: ["templateVersion", templateName, versionName],
    queryFn: () => getTemplateVersionData(orgId, templateName, versionName),
    ...options,
  })
}

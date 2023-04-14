import { useQuery, UseQueryOptions } from "@tanstack/react-query"
import { getFile, getTemplateByName, getTemplateVersionByName } from "api/api"
import { TarReader } from "utils/tar"
import { createTemplateVersionFileTree } from "utils/templateVersion"

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
  const tarReader = new TarReader()
  await tarReader.readFile(tarFile)
  const fileTree = await createTemplateVersionFileTree(tarReader)

  return {
    template,
    version,
    fileTree,
    tarReader,
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

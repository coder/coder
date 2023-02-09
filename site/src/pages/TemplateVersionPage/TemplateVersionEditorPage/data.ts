import { useQuery } from "@tanstack/react-query"
import { getTemplateByName, getTemplateVersionByName } from "api/api"
import { getTemplateVersionFileTree } from "util/templateVersion"

const getTemplateVersionData = async (
  orgId: string,
  templateName: string,
  versionName: string,
) => {
  const [template, currentVersion] = await Promise.all([
    getTemplateByName(orgId, templateName),
    getTemplateVersionByName(orgId, templateName, versionName),
  ])

  const allowedExtensions = ["tf", "md", "Dockerfile"]
  const allowedFiles = ["Dockerfile"]
  const files = await getTemplateVersionFileTree(
    currentVersion,
    allowedExtensions,
    allowedFiles,
  )

  return {
    template,
    currentVersion,
    files,
  }
}

export const useTemplateVersionData = (
  orgId: string,
  templateName: string,
  versionName: string,
) => {
  return useQuery({
    queryKey: ["templateVersion", templateName, versionName],
    queryFn: () => getTemplateVersionData(orgId, templateName, versionName),
  })
}

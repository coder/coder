import { useQuery } from "@tanstack/react-query"
import { getTemplateByName, getTemplateVersionByName } from "api/api"
import { createTemplateVersionFileTree } from "util/templateVersion"

const getTemplateVersionData = async (
  orgId: string,
  templateName: string,
  versionName: string,
) => {
  const [template, currentVersion] = await Promise.all([
    getTemplateByName(orgId, templateName),
    getTemplateVersionByName(orgId, templateName, versionName),
  ])
  const fileTree = await createTemplateVersionFileTree(currentVersion)

  return {
    template,
    currentVersion,
    fileTree,
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

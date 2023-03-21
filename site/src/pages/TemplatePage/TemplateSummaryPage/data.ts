import { useQuery } from "@tanstack/react-query"
import {
  getTemplateVersionResources,
  getTemplateVersions,
  getTemplateDAUs,
} from "api/api"

const fetchTemplateSummary = async (
  templateId: string,
  activeVersionId: string,
) => {
  const [resources, versions, daus] = await Promise.all([
    getTemplateVersionResources(activeVersionId),
    getTemplateVersions(templateId),
    getTemplateDAUs(templateId),
  ])

  return {
    resources,
    versions,
    daus,
  }
}

export const useTemplateSummaryData = (
  templateId: string,
  activeVersionId: string,
) => {
  return useQuery({
    queryKey: ["template", templateId, "summary"],
    queryFn: () => fetchTemplateSummary(templateId, activeVersionId),
  })
}

export type TemplateSummaryData = Awaited<
  ReturnType<typeof fetchTemplateSummary>
>

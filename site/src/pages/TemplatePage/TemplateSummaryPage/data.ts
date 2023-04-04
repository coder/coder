import { useQuery } from "@tanstack/react-query"
import { getTemplateVersionResources, getTemplateDAUs } from "api/api"

const fetchTemplateSummary = async (
  templateId: string,
  activeVersionId: string,
) => {
  const [resources, daus] = await Promise.all([
    getTemplateVersionResources(activeVersionId),
    getTemplateDAUs(templateId),
  ])

  return {
    resources,
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

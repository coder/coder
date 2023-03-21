import { useQuery } from "@tanstack/react-query"
import { getPreviousTemplateVersionByName } from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import { Loader } from "components/Loader/Loader"
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { useOrganizationId } from "hooks/useOrganizationId"
import { useTab } from "hooks/useTab"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { getTemplateVersionFiles } from "util/templateVersion"

const fetchTemplateFiles = async (
  organizationId: string,
  templateName: string,
  activeVersion: TemplateVersion,
) => {
  const previousVersion = await getPreviousTemplateVersionByName(
    organizationId,
    templateName,
    activeVersion.name,
  )
  const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] = []
  loadFilesPromises.push(getTemplateVersionFiles(activeVersion))
  if (previousVersion) {
    loadFilesPromises.push(getTemplateVersionFiles(previousVersion))
  }
  const [currentFiles, previousFiles] = await Promise.all(loadFilesPromises)
  return {
    currentFiles,
    previousFiles,
  }
}

const useTemplateFiles = (
  organizationId: string,
  templateName: string,
  activeVersion: TemplateVersion,
) =>
  useQuery({
    queryKey: ["templateFiles", templateName],
    queryFn: () =>
      fetchTemplateFiles(organizationId, templateName, activeVersion),
  })

const TemplateFilesPage: FC = () => {
  const { template, activeVersion } = useTemplateLayoutContext()
  const orgId = useOrganizationId()
  const tab = useTab("file", "0")
  const { data: templateFiles } = useTemplateFiles(
    orgId,
    template.name,
    activeVersion,
  )

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template?.name} · Source Code`)}</title>
      </Helmet>

      {templateFiles ? (
        <TemplateFiles
          currentFiles={templateFiles.currentFiles}
          previousFiles={templateFiles.previousFiles}
          tab={tab}
        />
      ) : (
        <Loader />
      )}
    </>
  )
}

export default TemplateFilesPage

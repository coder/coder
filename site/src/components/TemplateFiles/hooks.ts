import { TemplateVersion } from "api/typesGenerated";
import { useTab } from "hooks/useTab";
import { useEffect } from "react";
import { useQuery } from "react-query";
import {
  TemplateVersionFiles,
  getTemplateVersionFiles,
} from "utils/templateVersion";
import * as API from "api/api";

export const useFileTab = (templateFiles: TemplateVersionFiles | undefined) => {
  // Tabs The default tab is the tab that has main.tf but until we loads the
  // files and check if main.tf exists we don't know which tab is the default
  // one so we just use empty string
  const tab = useTab("file", "");
  const isLoaded = tab.value !== "";
  useEffect(() => {
    if (templateFiles && !isLoaded) {
      const terraformFileIndex = Object.keys(templateFiles).indexOf("main.tf");
      // If main.tf exists use the index if not just use the first tab
      tab.set(terraformFileIndex !== -1 ? terraformFileIndex.toString() : "0");
    }
  }, [isLoaded, tab, templateFiles]);

  return {
    ...tab,
    isLoaded,
  };
};

export const useTemplateFiles = (
  templateName: string,
  version: TemplateVersion | undefined,
) => {
  return useQuery({
    queryKey: ["templateFiles", templateName, version],
    queryFn: () => {
      if (!version) {
        return;
      }
      return getTemplateFilesWithDiff(templateName, version);
    },
    enabled: version !== undefined,
  });
};

const getTemplateFilesWithDiff = async (
  templateName: string,
  version: TemplateVersion,
) => {
  const previousVersion = await API.getPreviousTemplateVersionByName(
    version.organization_id!,
    templateName,
    version.name,
  );
  const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] = [];
  loadFilesPromises.push(getTemplateVersionFiles(version.job.file_id));
  if (previousVersion) {
    loadFilesPromises.push(
      getTemplateVersionFiles(previousVersion.job.file_id),
    );
  }
  const [currentFiles, previousFiles] = await Promise.all(loadFilesPromises);
  return {
    currentFiles,
    previousFiles,
  };
};

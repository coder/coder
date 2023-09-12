import * as API from "api/api";

export const buildInfo = () => {
  return {
    queryKey: ["buildInfo"],
    queryFn: fetchBuildInfo,
  };
};

const fetchBuildInfo = async () => {
  // Build info is injected by the Coder server into the HTML document.
  const buildInfo = document.querySelector("meta[property=build-info]");
  if (buildInfo) {
    const rawContent = buildInfo.getAttribute("content");
    try {
      return JSON.parse(rawContent as string);
    } catch (e) {
      console.warn("Failed to parse build info from document", e);
    }
  }

  return API.getBuildInfo();
};

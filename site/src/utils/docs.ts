import { getStaticBuildInfo } from "./buildInfo";

const DEFAULT_DOCS_URL = "https://coder.com/docs";

// Add cache to avoid DOM reading all the time
let CACHED_DOCS_URL: string | undefined;

export const docs = (path: string) => {
  return `${getBaseDocsURL()}${path}`;
};

const getBaseDocsURL = () => {
  if (!CACHED_DOCS_URL) {
    const docsUrl = document
      .querySelector<HTMLMetaElement>('meta[property="docs-url"]')
      ?.getAttribute("content");

    CACHED_DOCS_URL = docsUrl && isURL(docsUrl) ? docsUrl : DEFAULT_DOCS_URL;

    // If we can get the specific version, we want to include that in docs links
    const version = getStaticBuildInfo()?.version.split("-")[0];
    if (version) {
      CACHED_DOCS_URL = `${CACHED_DOCS_URL}/@${version}`;
    }
  }
  return CACHED_DOCS_URL;
};

const isURL = (value: string) => {
  try {
    new URL(value);
    return true;
  } catch {
    return false;
  }
};

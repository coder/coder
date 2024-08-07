import { getStaticBuildInfo } from "./buildInfo";

function defaultDocsUrl(): string {
  const docsUrl = "https://coder.com/docs";
  // If we can get the specific version, we want to include that in default docs URL.
  const version = getStaticBuildInfo()?.version.split("-")[0];
  return version ? `${docsUrl}/@${version}` : docsUrl;
}

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

    const isValidDocsURL = docsUrl && isURL(docsUrl);
    CACHED_DOCS_URL = isValidDocsURL ? docsUrl : defaultDocsUrl();
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

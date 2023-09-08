const DEFAULT_DOCS_URL = "https://coder.com/docs/v2/latest";

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
    CACHED_DOCS_URL = isValidDocsURL ? docsUrl : DEFAULT_DOCS_URL;
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

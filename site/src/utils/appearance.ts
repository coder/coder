export const getApplicationName = (): string => {
  const c = document.head
    .querySelector(`meta[name=application-name]`)
    ?.getAttribute("content");
  // Fallback to "Coder" if the application name is not available for some reason.
  // We need to check if the content does not look like {{ .ApplicationName}}
  // as it means that Coder is running in development mode (port :8080).
  return c && !c.startsWith("{{ .") ? c : "Coder";
};

export const getLogoURL = (): string => {
  const c = document.head
    .querySelector(`meta[property=logo-url]`)
    ?.getAttribute("content");
  return c && !c.startsWith("{{ .") ? c : "";
};

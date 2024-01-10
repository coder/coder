import * as TypesGen from "api/typesGenerated";

export const createAppLinkHref = (
  protocol: string,
  preferredPathBase: string,
  appsHost: string,
  appSlug: string,
  username: string,
  workspace: TypesGen.Workspace,
  agent: TypesGen.WorkspaceAgent,
  app: TypesGen.WorkspaceApp,
): string => {
  if (app.external) {
    return app.url;
  }

  // The backend redirects if the trailing slash isn't included, so we add it
  // here to avoid extra roundtrips.
  let href = `${preferredPathBase}/@${username}/${workspace.name}.${
    agent.name
  }/apps/${encodeURIComponent(appSlug)}/`;
  if (app.command) {
    // Terminal links are relative. The terminal page knows how
    // to select the correct workspace proxy for the websocket
    // connection.
    href = `/@${username}/${workspace.name}.${
      agent.name
    }/terminal?command=${encodeURIComponent(app.command)}`;
  }

  if (appsHost && app.subdomain && app.subdomain_name) {
    href = `${protocol}//${appsHost}/`.replace("*", app.subdomain_name);
  }
  return href;
};

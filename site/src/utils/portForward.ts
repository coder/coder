export const portForwardURL = (
  host: string,
  port: number,
  agentName: string,
  workspaceName: string,
  username: string,
): string => {
  const { location } = window;

  const subdomain = `${
    isNaN(port) ? 3000 : port
  }--${agentName}--${workspaceName}--${username}`;
  return `${location.protocol}//${host}`.replace("*", subdomain);
};

// openMaybePortForwardedURL tries to open the provided URI through the
// port-forwarded URL if it is localhost, otherwise opens it normally.
export const openMaybePortForwardedURL = (
  uri: string,
  proxyHost?: string,
  agentName?: string,
  workspaceName?: string,
  username?: string,
) => {
  const open = (uri: string) => {
    // Copied from: https://github.com/xtermjs/xterm.js/blob/master/addons/xterm-addon-web-links/src/WebLinksAddon.ts#L23
    const newWindow = window.open();
    if (newWindow) {
      try {
        newWindow.opener = null;
      } catch {
        // no-op, Electron can throw
      }
      newWindow.location.href = uri;
    } else {
      console.warn("Opening link blocked as opener could not be cleared");
    }
  };

  if (!agentName || !workspaceName || !username || !proxyHost) {
    open(uri);
    return;
  }

  try {
    const url = new URL(uri);
    const localHosts = ["0.0.0.0", "127.0.0.1", "localhost"];
    if (!localHosts.includes(url.hostname)) {
      open(uri);
      return;
    }
    open(
      portForwardURL(
        proxyHost,
        parseInt(url.port),
        agentName,
        workspaceName,
        username,
      ) + url.pathname,
    );
  } catch (ex) {
    open(uri);
  }
};

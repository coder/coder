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

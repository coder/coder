import dayjs from "dayjs";
import type { Workspace } from "api/typesGenerated";

export type WorkspaceActivityStatus =
  | "ready"
  | "connected"
  | "inactive"
  | "notConnected"
  | "notRunning";

export function getWorkspaceActivityStatus(
  workspace: Workspace,
): WorkspaceActivityStatus {
  const builtAt = dayjs(workspace.latest_build.created_at);
  const usedAt = dayjs(workspace.last_used_at);
  const now = dayjs();

  if (workspace.latest_build.status !== "running") {
    return "notRunning";
  }

  // This needs to compare to `usedAt` instead of `now`, because the "grace period" for
  // marking a workspace as "Connected" is a lot longer. If you compared `builtAt` to `now`,
  // you could end up switching from "Ready" to "Connected" without ever actually connecting.
  const isBuiltRecently = builtAt.isAfter(usedAt.subtract(1, "second"));
  // By default, agents report connection stats every 30 seconds, so 2 minutes should be
  // plenty. Disconnection will be reflected relatively-quickly
  const isUsedRecently = usedAt.isAfter(now.subtract(2, "minute"));

  // If the build is still "fresh", it'll be a while before the `last_used_at` gets bumped in
  // a significant way by the agent, so just label it as ready instead of connected.
  // Wait until `last_used_at` is after the time that the build finished, _and_ still
  // make sure to check that it's recent, so that we don't show "Ready" indefinitely.
  if (isUsedRecently && isBuiltRecently && workspace.health.healthy) {
    return "ready";
  }

  if (isUsedRecently) {
    return "connected";
  }

  // TODO: It'd be nice if we could differentiate between "connected but inactive" and
  // "not connected", but that will require some relatively substantial backend work.
  return "inactive";
}

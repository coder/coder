import type { BuildInfoResponse } from "api/typesGenerated";

// sendDeploymentEvent sends a CORs payload to coder.com
// to track a deployment event.
export const sendDeploymentEvent = (
  buildInfo: BuildInfoResponse,
  payload: {
    type: "deployment_setup" | "deployment_login";
    user_id?: string;
  },
) => {
  if (typeof navigator === "undefined" || !navigator.sendBeacon) {
    // It's fine if we don't report this, it's not required!
    return;
  }
  if (!buildInfo.telemetry) {
    return;
  }
  navigator.sendBeacon(
    "https://coder.com/api/track-deployment",
    new Blob(
      [
        JSON.stringify({
          ...payload,
          deployment_id: buildInfo.deployment_id,
        }),
      ],
      {
        type: "application/json",
      },
    ),
  );
};

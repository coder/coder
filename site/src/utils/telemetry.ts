// sendDeploymentEvent sends a CORs payload to coder.com
// to track a deployment event.
export const sendDeploymentEvent = (payload: {
  type: "deployment_setup" | "deployment_login";
  deployment_id: string;
  user_id?: string;
}) => {
  if (typeof navigator === "undefined" || !navigator.sendBeacon) {
    // It's fine if we don't report this, it's not required!
    return;
  }
  navigator.sendBeacon(
    "https://coder.com/api/track-deployment",
    new Blob([JSON.stringify(payload)], {
      type: "application/json",
    }),
  );
};

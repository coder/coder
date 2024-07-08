import type { ProvisionerJob } from "api/typesGenerated";

export const getPendingStatusLabel = (
  provisionerJob?: ProvisionerJob,
): string => {
  if (!provisionerJob || provisionerJob.queue_size === 0) {
    return "Pending";
  }
  return "Position in queue: " + provisionerJob.queue_position;
};

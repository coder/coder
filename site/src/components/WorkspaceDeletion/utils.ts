import { Workspace } from "api/typesGenerated";

// This const dictates how far out we alert the user that a workspace
// has an impending deletion (due to template.InactivityTTL being set)
const IMPENDING_DELETION_DISPLAY_THRESHOLD = 14; // 14 days

/**
 * Returns a boolean indicating if an impending deletion indicator should be
 * displayed in the UI. Impending deletions are configured by setting the
 * Template.InactivityTTL
 * @param {TypesGen.Workspace} workspace
 * @returns {boolean}
 */
export const displayDormantDeletion = (
  workspace: Workspace,
  allowAdvancedScheduling: boolean,
) => {
  const today = new Date();
  if (!workspace.deleting_at || !allowAdvancedScheduling) {
    return false;
  }
  return (
    new Date(workspace.deleting_at) <=
    new Date(
      today.setDate(today.getDate() + IMPENDING_DELETION_DISPLAY_THRESHOLD),
    )
  );
};

import { Workspace } from "api/typesGenerated";
import { displayDormantDeletion } from "./utils";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import styled from "@emotion/styled";
import { Theme as MaterialUITheme } from "@mui/material/styles";

export const DormantDeletionText = ({
  workspace,
}: {
  workspace: Workspace;
}): JSX.Element | null => {
  const { entitlements, experiments } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions");

  if (
    !displayDormantDeletion(
      workspace,
      allowAdvancedScheduling,
      allowWorkspaceActions,
    )
  ) {
    return null;
  }
  return <StyledSpan role="status">Impending deletion</StyledSpan>;
};

const StyledSpan = styled.span<{ theme?: MaterialUITheme }>`
  color: ${(props) => props.theme.palette.warning.light};
  font-weight: 600;
`;

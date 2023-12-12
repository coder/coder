import RefreshIcon from "@mui/icons-material/Refresh";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import Skeleton from "@mui/material/Skeleton";
import Link from "@mui/material/Link";
import { type FC } from "react";
import { useQuery } from "react-query";
import { type Interpolation, type Theme, css, useTheme } from "@emotion/react";
import { templateVersion } from "api/queries/templates";
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";

export const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText:
    "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update",
};

interface TooltipProps {
  onUpdateVersion: () => void;
  latestVersionId: string;
  templateName: string;
  ariaLabel?: string;
}

export const WorkspaceOutdatedTooltip: FC<TooltipProps> = ({
  onUpdateVersion,
  ariaLabel,
  latestVersionId,
  templateName,
}) => {
  const { data: activeVersion } = useQuery(templateVersion(latestVersionId));
  const theme = useTheme();

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconStyles={styles.icon}
      buttonStyles={styles.button}
    >
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>

      <div css={styles.container}>
        <div>
          <div css={styles.bold}>New version</div>
          <div>
            {activeVersion ? (
              <Link
                href={`/templates/${templateName}/versions/${activeVersion.name}`}
                target="_blank"
                css={{ color: theme.palette.primary.light }}
              >
                {activeVersion.name}
              </Link>
            ) : (
              <Skeleton variant="text" height={20} width={100} />
            )}
          </div>
        </div>

        <div>
          <div css={styles.bold}>Message</div>
          <div>
            {activeVersion ? (
              activeVersion.message === "" ? (
                "No message"
              ) : (
                activeVersion.message
              )
            ) : (
              <Skeleton variant="text" height={20} width={150} />
            )}
          </div>
        </div>
      </div>

      <HelpTooltipLinksGroup>
        <HelpTooltipAction
          icon={RefreshIcon}
          onClick={onUpdateVersion}
          ariaLabel={ariaLabel}
        >
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  );
};

const styles = {
  icon: (theme) => ({
    color: theme.experimental.roles.notice.outline,
  }),

  button: css`
    opacity: 1;

    &:hover {
      opacity: 1;
    }
  `,

  container: {
    display: "flex",
    flexDirection: "column",
    gap: 8,
    paddingTop: 8,
    paddingBottom: 8,
    fontSize: 13,
  },

  bold: (theme) => ({
    color: theme.palette.text.primary,
    fontWeight: 600,
  }),
} satisfies Record<string, Interpolation<Theme>>;

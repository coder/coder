import RefreshIcon from "@mui/icons-material/Refresh";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import Skeleton from "@mui/material/Skeleton";
import Link from "@mui/material/Link";
import { type FC } from "react";
import { useQuery } from "react-query";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { templateVersion } from "api/queries/templates";
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipContent,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { usePopover } from "components/Popover/Popover";

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

export const WorkspaceOutdatedTooltip: FC<TooltipProps> = (props) => {
  return (
    <HelpTooltip>
      <HelpTooltipTrigger
        size="small"
        aria-label="More info"
        hoverEffect={false}
      >
        <InfoIcon css={styles.icon} />
      </HelpTooltipTrigger>

      <WorkspaceOutdatedTooltipContent {...props} />
    </HelpTooltip>
  );
};

export const WorkspaceOutdatedTooltipContent: FC<TooltipProps> = ({
  onUpdateVersion,
  ariaLabel,
  latestVersionId,
  templateName,
}) => {
  const popover = usePopover();
  const { data: activeVersion } = useQuery({
    ...templateVersion(latestVersionId),
    enabled: popover.isOpen,
  });
  const theme = useTheme();

  return (
    <HelpTooltipContent>
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>

      <div css={styles.container}>
        <div css={{ lineHeight: "1.6" }}>
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

        <div css={{ lineHeight: "1.6" }}>
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
    </HelpTooltipContent>
  );
};

const styles = {
  icon: (theme) => ({
    color: theme.experimental.roles.notice.outline,
  }),

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

import type { Interpolation, Theme } from "@emotion/react";
import Checkbox from "@mui/material/Checkbox";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import type { BannerConfig } from "api/typesGenerated";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";

interface NotificationBannerItemProps {
  enabled: boolean;
  backgroundColor?: string;
  message?: string;
  onUpdate: (banner: Partial<BannerConfig>) => Promise<void>;
  onEdit: () => void;
  onDelete: () => void;
}

export const NotificationBannerItem: FC<NotificationBannerItemProps> = ({
  enabled,
  backgroundColor = "#004852",
  message,
  onUpdate,
  onEdit,
  onDelete,
}) => {
  return (
    <TableRow>
      <TableCell>
        <Checkbox
          size="small"
          checked={enabled}
          onClick={() => void onUpdate({ enabled: !enabled })}
        />
      </TableCell>

      <TableCell css={!enabled && styles.disabled}>
        {message || <em>No message</em>}
      </TableCell>

      <TableCell>
        <div css={styles.colorSample} style={{ backgroundColor }}></div>
      </TableCell>

      <TableCell>
        <MoreMenu>
          <MoreMenuTrigger>
            <ThreeDotsButton />
          </MoreMenuTrigger>
          <MoreMenuContent>
            <MoreMenuItem onClick={() => onEdit()}>Edit&hellip;</MoreMenuItem>
            <MoreMenuItem onClick={() => onDelete()} danger>
              Delete&hellip;
            </MoreMenuItem>
          </MoreMenuContent>
        </MoreMenu>
      </TableCell>
    </TableRow>
  );
};

const styles = {
  disabled: (theme) => ({
    color: theme.roles.inactive.fill.outline,
  }),

  colorSample: {
    width: 24,
    height: 24,
    borderRadius: 4,
  },
} satisfies Record<string, Interpolation<Theme>>;

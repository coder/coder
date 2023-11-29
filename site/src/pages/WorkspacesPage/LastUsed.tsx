import { type FC } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useTheme } from "@emotion/react";
import { Stack } from "components/Stack/Stack";
import { colors } from "theme/colors";
import { upperFirst } from "lodash";
import {
  HelpTooltip,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";

dayjs.extend(relativeTime);

type CircleProps = {
  color: string;
  variant?: "solid" | "outlined";
};

const Circle: FC<CircleProps> = ({ color, variant = "solid" }) => {
  return (
    <div
      aria-hidden
      css={{
        width: 8,
        height: 8,
        backgroundColor: variant === "solid" ? color : undefined,
        border: variant === "outlined" ? `1px solid ${color}` : undefined,
        borderRadius: 9999,
      }}
    />
  );
};

interface LastUsedProps {
  lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
  const theme = useTheme();
  const t = dayjs(lastUsedAt);
  const now = dayjs();
  let message = t.fromNow();
  let circle = (
    <Circle color={theme.palette.text.secondary} variant="outlined" />
  );

  if (t.isAfter(now.subtract(1, "hour"))) {
    circle = <Circle color={colors.green[9]} />;
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
  } else if (t.isAfter(now.subtract(3, "day"))) {
    circle = <Circle color={theme.palette.text.secondary} />;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    circle = <Circle color={theme.palette.warning.light} />;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    circle = <Circle color={colors.red[10]} />;
  } else {
    // color = theme.palette.error.light
    message = "Never";
  }

  return (
    <Stack
      css={{ color: theme.palette.text.secondary }}
      direction="row"
      spacing={1}
      alignItems="center"
    >
      {circle}
      <span data-chromatic="ignore">{message}</span>
    </Stack>
  );
};

interface DormantProps {
  deletingAt?: string;
}

export const DeletingAt: FC<DormantProps> = ({ deletingAt }) => {
  const theme = useTheme();
  let message = "Never";

  let circle = (
    <Circle color={theme.palette.text.secondary} variant="outlined" />
  );

  const formatDate = (dateStr: string): string => {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      month: "long",
      day: "numeric",
      year: "numeric",
    });
  };

  if (deletingAt) {
    const t = dayjs(deletingAt);
    const now = dayjs();
    message = upperFirst(t.fromNow());
    if (t.isAfter(now.add(1, "month"))) {
      circle = <Circle color={colors.green[9]} />;
    } else if (
      t.isAfter(now.add(7, "day")) &&
      t.isBefore(now.add(1, "month"))
    ) {
      message = `In ${t.diff(now, "day")} days`;
      circle = <Circle color={theme.palette.warning.light} />;
    } else if (t.isBefore(now.add(7, "day"))) {
      circle = <Circle color={colors.red[10]} />;
    }
  }

  return (
    <Stack
      css={{ color: theme.palette.text.secondary }}
      direction="row"
      spacing={1}
      alignItems="center"
    >
      {circle}
      <span data-chromatic="ignore">{message}</span>
      <HelpTooltip>
        <HelpTooltipText>
          {message === "Never"
            ? "The template for this workspace has not enabled auto-deletion"
            : deletingAt && `Deleting on ${formatDate(deletingAt)}`}
        </HelpTooltipText>
      </HelpTooltip>
    </Stack>
  );
};

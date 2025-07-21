import { Button } from "components/Button/Button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Link } from "components/Link/Link";
import { Stack } from "components/Stack/Stack";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { docs } from "utils/docs";
import MuiLink from "@mui/material/Link";
import { type Interpolation, type Theme } from "@emotion/react";
import dayjs from "dayjs";

interface ManagedAgentsConsumptionProps {
  usage: number;
  included: number;
  limit: number;
  startDate: string;
  endDate: string;
  enabled?: boolean;
}

export const ManagedAgentsConsumption: FC<ManagedAgentsConsumptionProps> = ({
  usage,
  included,
  limit,
  startDate,
  endDate,
  enabled = true,
}) => {

  if (!enabled) {
    return (
      <div css={styles.disabledRoot}>
        <Stack alignItems="center" spacing={1}>
          <Stack alignItems="center" spacing={0.5}>
            <span css={styles.disabledTitle}>
              Managed AI Agent Feature Disabled
            </span>
            <span css={styles.disabledDescription}>
              The managed AI agent feature is not included in your current license.
              Contact{" "}
              <MuiLink href="mailto:sales@coder.com">sales</MuiLink> to
              upgrade your license and unlock this feature.
            </span>
          </Stack>
        </Stack>
      </div>
    );
  }

  const usagePercentage = Math.min((usage / limit) * 100, 100);
  const includedPercentage = Math.min((included / limit) * 100, 100);
  const remainingPercentage = Math.max(100 - includedPercentage, 0);

  return (
    <section className="border border-solid rounded">
      <div className="p-4">
        <Collapsible>
          <header className="flex flex-col gap-2 items-start">
            <h3 className="text-md m-0 font-medium">
              Managed agents consumption
            </h3>

            <CollapsibleTrigger asChild>
              <Button
                className={`
                  h-auto p-0 border-0 bg-transparent font-medium text-content-secondary
                  hover:bg-transparent hover:text-content-primary
                  [&[data-state=open]_svg]:rotate-90
                `}
              >
                <ChevronRightIcon />
                How we calculate managed agent consumption
              </Button>
            </CollapsibleTrigger>
          </header>

          <CollapsibleContent
            className={`
              pt-2 pl-7 pr-5 space-y-4 font-medium max-w-[720px]
              text-sm text-content-secondary
              [&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
            `}
          >
            <p>
              Managed agents are counted based on the amount of started workspaces with an AI agent.
            </p>
            <ul>
              <li className="flex items-center gap-2">
                <div
                  className="rounded-[2px] bg-highlight-green size-3 inline-block"
                  aria-label="Legend for current usage in the chart"
                />
                Amount of started workspaces with an AI agent.
              </li>
              <li className="flex items-center gap-2">
                <div
                  className="rounded-[2px] bg-content-disabled size-3 inline-block"
                  aria-label="Legend for included allowance in the chart"
                />
                Included allowance from your current license plan.
              </li>
              <li className="flex items-center gap-2">
                <div
                  className="size-3 inline-flex items-center justify-center"
                  aria-label="Legend for total limit in the chart"
                >
                  <div className="w-full border-b-1 border-t-1 border-dashed border-content-disabled" />
                </div>
                Total limit after which the feature will be disabled.
              </li>
            </ul>
          </CollapsibleContent>
        </Collapsible>
      </div>

      <div className="p-6 border-0 border-t border-solid">
        <div className="flex justify-between text-sm text-content-secondary mb-4">
          <span>{startDate ? dayjs(startDate).format("MMMM D, YYYY") : ""}</span>
          <span>{endDate ? dayjs(endDate).format("MMMM D, YYYY") : ""}</span>
        </div>

        <div className="relative h-6 bg-surface-secondary rounded overflow-hidden">
          <div
            className="absolute top-0 left-0 h-full bg-highlight-green transition-all duration-300"
            style={{ width: `${usagePercentage}%` }}
          />

          <div
            className="absolute top-0 h-full bg-content-disabled opacity-30"
            style={{
              left: `${includedPercentage}%`,
              width: `${remainingPercentage}%`,
            }}
          />
        </div>

        <div className="relative flex justify-between mt-4 text-sm">
          <div className="flex flex-col items-start">
            <span className="text-content-secondary">Actual:</span>
            <span className="font-medium">{usage.toLocaleString()}</span>
          </div>

          <div
            className="absolute flex flex-col items-center transform -translate-x-1/2"
            style={{ left: `${Math.max(Math.min(includedPercentage, 90), 10)}%` }}
          >
            <span className="text-content-secondary">Included:</span>
            <span className="font-medium">{included.toLocaleString()}</span>
          </div>

          <div className="flex flex-col items-end">
            <span className="text-content-secondary">Limit:</span>
            <span className="font-medium">{limit.toLocaleString()}</span>
          </div>
        </div>
      </div>
    </section>
  );
};

const styles = {
  disabledTitle: {
    fontSize: 16,
  },

  disabledRoot: (theme) => ({
    minHeight: 240,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
    padding: 48,
  }),

  disabledDescription: (theme) => ({
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: 464,
    marginTop: 8,
  }),
} satisfies Record<string, Interpolation<Theme>>;

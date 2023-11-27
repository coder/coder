import { watchAgentMetadata } from "api/api";
import type {
  WorkspaceAgent,
  WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import {
  createContext,
  FC,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import Skeleton from "@mui/material/Skeleton";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import Tooltip from "@mui/material/Tooltip";
import Box, { BoxProps } from "@mui/material/Box";
import { type Interpolation, type Theme } from "@emotion/react";

type ItemStatus = "stale" | "valid" | "loading";

export const WatchAgentMetadataContext = createContext(watchAgentMetadata);

const MetadataItem: FC<{ item: WorkspaceAgentMetadata }> = ({ item }) => {
  if (item.result === undefined) {
    throw new Error("Metadata item result is undefined");
  }
  if (item.description === undefined) {
    throw new Error("Metadata item description is undefined");
  }

  const staleThreshold = Math.max(
    item.description.interval + item.description.timeout * 2,
    // In case there is intense backpressure, we give a little bit of slack.
    5,
  );

  const status: ItemStatus = (() => {
    const year = dayjs(item.result.collected_at).year();
    if (year <= 1970 || isNaN(year)) {
      return "loading";
    }
    // There is a special circumstance for metadata with `interval: 0`. It is
    // expected that they run once and never again, so never display them as
    // stale.
    if (item.result.age > staleThreshold && item.description.interval > 0) {
      return "stale";
    }
    return "valid";
  })();

  // Stale data is as good as no data. Plus, we want to build confidence in our
  // users that what's shown is real. If times aren't correctly synced this
  // could be buggy. But, how common is that anyways?
  const value =
    status === "loading" ? (
      <Skeleton width={65} height={12} variant="text" css={styles.skeleton} />
    ) : status === "stale" ? (
      <Tooltip title="This data is stale and no longer up to date">
        <StaticWidth css={[styles.metadataValue, styles.metadataStale]}>
          {item.result.value}
        </StaticWidth>
      </Tooltip>
    ) : (
      <StaticWidth
        css={[
          styles.metadataValue,
          item.result.error.length === 0
            ? styles.metadataValueSuccess
            : styles.metadataValueError,
        ]}
      >
        {item.result.value}
      </StaticWidth>
    );

  return (
    <div css={styles.metadata}>
      <div css={styles.metadataLabel}>{item.description.display_name}</div>
      <Box>{value}</Box>
    </div>
  );
};

export interface AgentMetadataViewProps {
  metadata: WorkspaceAgentMetadata[];
}

export const AgentMetadataView: FC<AgentMetadataViewProps> = ({ metadata }) => {
  if (metadata.length === 0) {
    return <></>;
  }
  return (
    <div css={styles.root}>
      <Stack alignItems="baseline" direction="row" spacing={6}>
        {metadata.map((m) => {
          if (m.description === undefined) {
            throw new Error("Metadata item description is undefined");
          }
          return <MetadataItem key={m.description.key} item={m} />;
        })}
      </Stack>
    </div>
  );
};

export const AgentMetadata: FC<{
  agent: WorkspaceAgent;
  storybookMetadata?: WorkspaceAgentMetadata[];
}> = ({ agent, storybookMetadata }) => {
  const [metadata, setMetadata] = useState<
    WorkspaceAgentMetadata[] | undefined
  >(undefined);
  const watchAgentMetadata = useContext(WatchAgentMetadataContext);

  useEffect(() => {
    if (storybookMetadata !== undefined) {
      setMetadata(storybookMetadata);
      return;
    }

    let timeout: NodeJS.Timeout | undefined = undefined;

    const connect = (): (() => void) => {
      const source = watchAgentMetadata(agent.id);

      source.onerror = (e) => {
        console.error("received error in watch stream", e);
        setMetadata(undefined);
        source.close();

        timeout = setTimeout(() => {
          connect();
        }, 3000);
      };

      source.addEventListener("data", (e) => {
        const data = JSON.parse(e.data);
        setMetadata(data);
      });
      return () => {
        if (timeout !== undefined) {
          clearTimeout(timeout);
        }
        source.close();
      };
    };
    return connect();
  }, [agent.id, watchAgentMetadata, storybookMetadata]);

  if (metadata === undefined) {
    return (
      <div css={styles.root}>
        <AgentMetadataSkeleton />
      </div>
    );
  }

  return <AgentMetadataView metadata={metadata} />;
};

export const AgentMetadataSkeleton: FC = () => {
  return (
    <Stack alignItems="baseline" direction="row" spacing={6}>
      <div css={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>

      <div css={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>

      <div css={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>
    </Stack>
  );
};

const StaticWidth = (props: BoxProps) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Ignore this in storybook
    if (!ref.current || process.env.STORYBOOK === "true") {
      return;
    }

    const currentWidth = ref.current.getBoundingClientRect().width;
    ref.current.style.width = "auto";
    const autoWidth = ref.current.getBoundingClientRect().width;
    ref.current.style.width =
      autoWidth > currentWidth ? `${autoWidth}px` : `${currentWidth}px`;
  }, [props.children]);

  return <Box {...props} ref={ref} />;
};

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const styles = {
  root: (theme) => ({
    padding: "20px 32px",
    borderTop: `1px solid ${theme.palette.divider}`,
    overflowX: "auto",
    scrollPadding: "0 32px",
  }),

  metadata: {
    fontSize: 12,
    lineHeight: "normal",
    display: "flex",
    flexDirection: "column",
    gap: 4,
    overflow: "visible",

    // Because of scrolling
    "&:last-child": {
      paddingRight: 32,
    },
  },

  metadataLabel: (theme) => ({
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    fontWeight: 500,
  }),

  metadataValue: {
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    maxWidth: "16em",
    fontSize: 14,
  },

  metadataValueSuccess: (theme) => ({
    color: theme.palette.success.light,
  }),

  metadataValueError: (theme) => ({
    color: theme.palette.error.main,
  }),

  metadataStale: (theme) => ({
    color: theme.palette.text.disabled,
    cursor: "pointer",
  }),

  skeleton: {
    marginTop: 4,
  },

  inlineCommand: (theme) => ({
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: "inline-block",
    fontWeight: 600,
    margin: 0,
    borderRadius: 4,
    color: theme.palette.text.primary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

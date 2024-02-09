import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { type TemplateVersionExternalAuth } from "api/typesGenerated";
import { type ExternalAuthPollingState } from "../CreateWorkspacePage";
import { ExternalAuthItem } from "./ExternalAuthItem";

type ExternalAuthWallProps = {
  providers: TemplateVersionExternalAuth[];
  pollingState: ExternalAuthPollingState;
  onStartPolling: () => void;
};

export const ExternalAuthWall: FC<ExternalAuthWallProps> = ({
  providers,
  pollingState,
  onStartPolling,
}) => {
  return (
    <section css={styles.root}>
      <div css={styles.content}>
        <header css={styles.header}>
          <h3 css={styles.title}>External authentication</h3>
          <p css={styles.description}>
            To create a workspace using the selected template, please ensure you
            are connected with all the external services.
          </p>
        </header>

        <ul css={styles.providerList}>
          {providers.map((p) => (
            <ExternalAuthItem
              component="li"
              key={p.id}
              provider={p}
              isPolling={pollingState === "polling"}
              onStartPolling={onStartPolling}
            />
          ))}
        </ul>
      </div>
    </section>
  );
};

const styles = {
  root: (theme) => ({
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    padding: 48,
    minHeight: 460,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    lineHeight: "1.5",
  }),

  header: {
    textAlign: "center",
    // Better text distribution
    maxWidth: 324,
    margin: "auto",
  },

  content: {
    maxWidth: 380,
  },

  title: {
    fontSize: 20,
    fontWeight: 400,
    margin: 0,
    lineHeight: "1.2",
  },

  description: (theme) => ({
    margin: 0,
    marginTop: 12,
    fontSize: 14,
    color: theme.palette.text.secondary,
  }),

  providerList: {
    listStyle: "none",
    padding: 0,
    margin: 0,
    display: "flex",
    flexDirection: "column",
    gap: 8,
    marginTop: 24,
  },
} as Record<string, Interpolation<Theme>>;

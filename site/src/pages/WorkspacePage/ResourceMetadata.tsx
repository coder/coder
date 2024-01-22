import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { SensitiveValue } from "components/Resources/SensitiveValue";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { WorkspaceResource } from "api/typesGenerated";
import { Children, FC, HTMLAttributes, PropsWithChildren } from "react";
import { Interpolation, Theme } from "@emotion/react";

type ResourceMetadataProps = Omit<HTMLAttributes<HTMLElement>, "resource"> & {
  resource: WorkspaceResource;
};

export const ResourceMetadata: FC<ResourceMetadataProps> = ({
  resource,
  ...headerProps
}) => {
  const metadata = resource.metadata ? [...resource.metadata] : [];

  if (resource.daily_cost > 0) {
    metadata.push({
      key: "Daily cost",
      value: resource.daily_cost.toString(),
      sensitive: false,
    });
  }

  if (metadata.length === 0) {
    return null;
  }

  return (
    <header css={styles.root} {...headerProps}>
      {metadata.map((meta) => {
        return (
          <div css={styles.item} key={meta.key}>
            <div css={styles.value}>
              {meta.sensitive ? (
                <SensitiveValue value={meta.value} />
              ) : (
                <MemoizedInlineMarkdown components={{ p: MetaValue }}>
                  {meta.value}
                </MemoizedInlineMarkdown>
              )}
            </div>
            <div css={styles.label}>{meta.key}</div>
          </div>
        );
      })}
    </header>
  );
};

const MetaValue = ({ children }: PropsWithChildren) => {
  const childrenArray = Children.toArray(children);
  if (childrenArray.every((child) => typeof child === "string")) {
    return (
      <CopyableValue value={childrenArray.join("")}>{children}</CopyableValue>
    );
  }
  return <>{children}</>;
};

const styles = {
  root: (theme) => ({
    padding: 24,
    display: "flex",
    flexWrap: "wrap",
    gap: 48,
    rowGap: 24,
    marginBottom: 24,
    fontSize: 14,
    background: `linear-gradient(180deg, ${theme.palette.background.default} 0%, rgba(0, 0, 0, 0) 100%)`,
  }),

  item: () => ({
    lineHeight: "1.5",
  }),

  label: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  }),

  value: () => ({
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  }),
} satisfies Record<string, Interpolation<Theme>>;

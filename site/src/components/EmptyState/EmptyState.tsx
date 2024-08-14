import type { FC, HTMLAttributes, ReactNode } from "react";

export interface EmptyStateProps extends HTMLAttributes<HTMLDivElement> {
  /** Text Message to display, placed inside Typography component */
  message: string;
  /** Longer optional description to display below the message */
  description?: string | ReactNode;
  cta?: ReactNode;
  image?: ReactNode;
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 */
export const EmptyState: FC<EmptyStateProps> = ({
  message,
  description,
  cta,
  image,
  ...attrs
}) => {
  return (
    <div
      css={{
        overflow: "hidden",
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        textAlign: "center",
        minHeight: 360,
        padding: "80px 40px",
        position: "relative",
      }}
      {...attrs}
    >
      <h5 css={{ fontSize: 24, fontWeight: 500, margin: 0 }}>{message}</h5>
      {description && (
        <p
          css={(theme) => ({
            marginTop: 16,
            fontSize: 16,
            lineHeight: "140%",
            maxWidth: 480,
            color: theme.palette.text.secondary,
          })}
        >
          {description}
        </p>
      )}
      {cta && <div css={{ marginTop: 24 }}>{cta}</div>}
      {image}
    </div>
  );
};

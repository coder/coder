import Box from "@mui/material/Box";
import type { FC, ReactNode } from "react";

export interface EmptyStateProps {
  /** Text Message to display, placed inside Typography component */
  message: string;
  /** Longer optional description to display below the message */
  description?: string | React.ReactNode;
  cta?: ReactNode;
  className?: string;
  image?: ReactNode;
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 *
 * EmptyState's props extend the [Material UI Box component](https://material-ui.com/components/box/)
 * that you can directly pass props through to to customize the shape and layout of it.
 */
export const EmptyState: FC<React.PropsWithChildren<EmptyStateProps>> = (
  props,
) => {
  const { message, description, cta, image, ...boxProps } = props;

  return (
    <Box
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
      {...boxProps}
    >
      <h5 css={{ fontSize: 24, fontWeight: 400, margin: 0 }}>{message}</h5>
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
    </Box>
  );
};

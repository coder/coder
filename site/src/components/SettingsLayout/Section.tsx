import Typography from "@mui/material/Typography";
import { type FC, type ReactNode, type PropsWithChildren } from "react";
import { SectionAction } from "./SectionAction";
import { type Interpolation, type Theme } from "@emotion/react";

type SectionLayout = "fixed" | "fluid";

export interface SectionProps {
  // Useful for testing
  id?: string;
  title?: ReactNode | string;
  description?: ReactNode;
  toolbar?: ReactNode;
  alert?: ReactNode;
  layout?: SectionLayout;
  className?: string;
  children?: ReactNode;
}

type SectionFC = FC<PropsWithChildren<SectionProps>> & {
  Action: typeof SectionAction;
};

export const Section: SectionFC = ({
  id,
  title,
  description,
  toolbar,
  alert,
  className = "",
  children,
  layout = "fixed",
}) => {
  return (
    <section className={className} id={id} data-testid={id}>
      <div css={{ maxWidth: layout === "fluid" ? "100%" : 500 }}>
        {(title || description) && (
          <div css={styles.header}>
            <div>
              {title && (
                <Typography variant="h4" sx={{ fontSize: 24 }}>
                  {title}
                </Typography>
              )}
              {description && typeof description === "string" && (
                <Typography css={styles.description}>{description}</Typography>
              )}
              {description && typeof description !== "string" && (
                <div css={styles.description}>{description}</div>
              )}
            </div>
            {toolbar && <div>{toolbar}</div>}
          </div>
        )}
        {alert && <div css={{ marginBottom: 8 }}>{alert}</div>}
        {children}
      </div>
    </section>
  );
};

// Sub-components
Section.Action = SectionAction;

const styles = {
  header: {
    marginBottom: 24,
    display: "flex",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  description: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 16,
    marginTop: 4,
    lineHeight: "140%",
  }),
} satisfies Record<string, Interpolation<Theme>>;

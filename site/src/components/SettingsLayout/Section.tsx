import { makeStyles } from "@mui/styles"
import Typography from "@mui/material/Typography"
import { FC, ReactNode, PropsWithChildren } from "react"
import { SectionAction } from "./SectionAction"

type SectionLayout = "fixed" | "fluid"

export interface SectionProps {
  // Useful for testing
  id?: string
  title?: ReactNode | string
  description?: ReactNode
  toolbar?: ReactNode
  alert?: ReactNode
  layout?: SectionLayout
  className?: string
  children?: ReactNode
}

type SectionFC = FC<PropsWithChildren<SectionProps>> & {
  Action: typeof SectionAction
}

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
  const styles = useStyles({ layout })
  return (
    <section className={className} id={id} data-testid={id}>
      <div className={styles.inner}>
        {(title || description) && (
          <div className={styles.header}>
            <div>
              {title && (
                <Typography variant="h4" sx={{ fontSize: 24 }}>
                  {title}
                </Typography>
              )}
              {description && typeof description === "string" && (
                <Typography className={styles.description}>
                  {description}
                </Typography>
              )}
              {description && typeof description !== "string" && (
                <div className={styles.description}>{description}</div>
              )}
            </div>
            {toolbar && <div>{toolbar}</div>}
          </div>
        )}
        {alert && <div className={styles.alert}>{alert}</div>}
        {children}
      </div>
    </section>
  )
}

// Sub-components
Section.Action = SectionAction

const useStyles = makeStyles((theme) => ({
  inner: ({ layout }: { layout: SectionLayout }) => ({
    maxWidth: layout === "fluid" ? "100%" : 500,
  }),
  alert: {
    marginBottom: theme.spacing(1),
  },
  header: {
    marginBottom: theme.spacing(3),
    display: "flex",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  description: {
    color: theme.palette.text.secondary,
    fontSize: 16,
    marginTop: theme.spacing(0.5),
    lineHeight: "140%",
  },
}))

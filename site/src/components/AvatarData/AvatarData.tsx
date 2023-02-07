import { Avatar } from "components/Avatar/Avatar"
import { FC, PropsWithChildren } from "react"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"

export interface AvatarDataProps {
  title: string | JSX.Element
  subtitle?: string
  src?: string
  avatar?: React.ReactNode
}

export const AvatarData: FC<PropsWithChildren<AvatarDataProps>> = ({
  title,
  subtitle,
  src,
  avatar,
}) => {
  const styles = useStyles()

  if (!avatar) {
    avatar = <Avatar src={src}>{title}</Avatar>
  }

  return (
    <Stack spacing={1.5} direction="row" alignItems="center">
      {avatar}

      <Stack spacing={0}>
        <span className={styles.title}>{title}</span>
        {subtitle && <span className={styles.subtitle}>{subtitle}</span>}
      </Stack>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },

  subtitle: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    lineHeight: "140%",
    marginTop: 2,
    maxWidth: 540,
  },
}))
